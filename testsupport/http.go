package testsupport

import (
	"context"
	"net/http"
)

type DummyHttpClient struct {
	Response      *http.Response
	Error         error
	AssertRequest func(req *http.Request)
}

type DummyHttpDoer struct {
	*HttpClientDoer
	Client *DummyHttpClient
}

func (c *DummyHttpClient) Do(req *http.Request) (*http.Response, error) {
	if c.AssertRequest != nil {
		c.AssertRequest(req)
	}
	return c.Response, c.Error
}

func NewDummyHttpDoer() *DummyHttpDoer {
	client := &DummyHttpClient{}
	doer := &HttpClientDoer{HttpClient: client}
	return &DummyHttpDoer{HttpClientDoer: doer, Client: client}
}

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// HttpClientDoer implements HttpDoer
type HttpClientDoer struct {
	HttpClient HttpClient
}

// Do overrides Do method of the default goa client Doer. It's needed for mocking http clients in tests.
func (d *HttpClientDoer) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	return d.HttpClient.Do(req)
}
