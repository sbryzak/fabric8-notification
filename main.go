package main

import (
	"context"
	"net/http"

	"github.com/fabric8-services/fabric8-notification/app"
	"github.com/fabric8-services/fabric8-notification/auth"
	"github.com/fabric8-services/fabric8-notification/collector"
	goaclient "github.com/goadesign/goa/client"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/fabric8-services/fabric8-notification/configuration"
	"github.com/fabric8-services/fabric8-notification/controller"
	"github.com/fabric8-services/fabric8-notification/email"
	"github.com/fabric8-services/fabric8-notification/jsonapi"
	"github.com/fabric8-services/fabric8-notification/keycloak"
	"github.com/fabric8-services/fabric8-notification/template"
	"github.com/fabric8-services/fabric8-notification/token"
	"github.com/fabric8-services/fabric8-notification/validator"
	"github.com/fabric8-services/fabric8-notification/wit"

	witmiddleware "github.com/fabric8-services/fabric8-wit/goamiddleware"
	"github.com/fabric8-services/fabric8-wit/log"
	"github.com/goadesign/goa"
	"github.com/goadesign/goa/middleware"
	"github.com/goadesign/goa/middleware/gzip"
	goajwt "github.com/goadesign/goa/middleware/security/jwt"
	"github.com/sirupsen/logrus"

	goalogrus "github.com/goadesign/goa/logging/logrus"
)

func main() {

	// Initialized configuration
	config, err := configuration.GetData()
	if err != nil {
		logrus.Panic(nil, map[string]interface{}{
			"err": err,
		}, "failed to setup the configuration")
	}

	// Initialized developer mode flag for the logger
	log.InitializeLogger(config.IsLogJSON(), config.GetLogLevel())

	keycloakConfig := keycloak.Config{
		BaseURL: config.GetKeycloakURL(),
		Realm:   config.GetKeycloakRealm(),
	}

	publicKey, err := keycloak.GetPublicKey(keycloakConfig)
	if err != nil {
		log.Panic(nil, map[string]interface{}{
			"err": err,
		}, "failed to parse public token")
	}

	err = config.Validate()
	if err != nil {
		log.Panic(nil, map[string]interface{}{
			"err": err,
		}, "Missing required configuration")
	}

	witClient, err := wit.NewCachedClient(config.GetWITURL())
	if err != nil {
		log.Panic(nil, map[string]interface{}{
			"url": config.GetWITURL(),
			"err": err,
		}, "Could not create WIT client")
	}

	authClient, err := auth.NewCachedClient(config.GetAuthURL())
	if err != nil {
		log.Panic(nil, map[string]interface{}{
			"url": config.GetAuthURL(),
			"err": err,
		}, "could not create Auth client")
	}

	// Calling the Auth service to generate a fabric8 service account token.
	// This is needed to call /api/users/ID and 'see' the
	// email address even if the user has made it 'private'
	saService := token.NewFabric8ServiceAccountTokenClient(authClient, config.GetServiceAccountID(), config.GetServiceAccountSecret())
	saToken, err := saService.Get(context.Background())
	if err != nil {
		log.Panic(nil, map[string]interface{}{
			"err": err,
		}, "could not generate service account token")
	}

	// update the client with the signer.
	authClient.SetJWTSigner(&goaclient.JWTSigner{
		TokenSource: &goaclient.StaticTokenSource{
			StaticToken: &goaclient.StaticToken{
				Value: saToken,
			},
		},
	})

	sender, err := email.NewMandrillSender(config.GetMadrillAPIKey())
	if err != nil {
		log.Panic(nil, map[string]interface{}{
			"err": err,
		}, "Could not create Madrill Sender")
	}

	notifier := email.NewAsyncWorkerNotifier(sender, 10)

	resolvers := &collector.LocalRegistry{}
	resolvers.Register("workitem.create", collector.ConfiguredVars(config, collector.NewWorkItemResolver(authClient, witClient)), nil)
	resolvers.Register("workitem.update", collector.ConfiguredVars(config, collector.NewWorkItemResolver(authClient, witClient)), nil)
	resolvers.Register("comment.create", collector.ConfiguredVars(config, collector.NewCommentResolver(authClient, witClient)), nil)
	resolvers.Register("comment.update", collector.ConfiguredVars(config, collector.NewCommentResolver(authClient, witClient)), nil)
	resolvers.Register("user.email.update", collector.ConfiguredVars(config, collector.NewUserResolver(authClient)), validator.ValidateUser)
	resolvers.Register("invitation.team.noorg", collector.ConfiguredVars(config, collector.NewUserResolver(authClient)), nil)
	resolvers.Register("invitation.space.noorg", collector.ConfiguredVars(config, collector.NewUserResolver(authClient)), nil)

	typeRegistry := &template.AssetRegistry{}
	service := goa.New("notification")

	// Mount middleware
	service.WithLogger(goalogrus.New(log.Logger()))
	service.Use(middleware.RequestID())
	service.Use(gzip.Middleware(9))
	service.Use(jsonapi.ErrorHandler(service, true))
	service.Use(middleware.Recover())

	service.Use(witmiddleware.TokenContext(publicKey, nil, app.NewJWTSecurity()))
	service.Use(log.LogRequest(config.IsDeveloperModeEnabled()))
	app.UseJWTMiddleware(service, goajwt.New(publicKey, nil, app.NewJWTSecurity()))

	// Mount "status" controller
	statusCtrl := controller.NewStatusController(service)
	app.MountStatusController(service, statusCtrl)

	// Mount "notification" controller
	notifyCtrl := controller.NewNotifyController(service, resolvers, typeRegistry, notifier)
	app.MountNotifyController(service, notifyCtrl)

	log.Logger().Infoln("Git Commit SHA: ", controller.Commit)
	log.Logger().Infoln("UTC Build Time: ", controller.BuildTime)
	log.Logger().Infoln("UTC Start Time: ", controller.StartTime)
	log.Logger().Infoln("Dev mode:       ", config.IsDeveloperModeEnabled())

	http.Handle("/api/", service.Mux)
	http.Handle("/favicon.ico", http.NotFoundHandler())

	// Start/mount metrics http
	if config.GetHTTPAddress() == config.GetMetricsHTTPAddress() {
		http.Handle("/metrics", prometheus.Handler())
	} else {
		go func(metricAddress string) {
			mx := http.NewServeMux()
			mx.Handle("/metrics", prometheus.Handler())
			if err := http.ListenAndServe(metricAddress, mx); err != nil {
				log.Error(nil, map[string]interface{}{
					"addr": metricAddress,
					"err":  err,
				}, "unable to connect to metrics server")
				service.LogError("startup", "err", err)
			}
		}(config.GetMetricsHTTPAddress())
	}

	// Start http
	if err := http.ListenAndServe(config.GetHTTPAddress(), nil); err != nil {
		log.Error(nil, map[string]interface{}{
			"addr": config.GetHTTPAddress(),
			"err":  err,
		}, "unable to connect to server")
		service.LogError("startup", "err", err)
	}
}
