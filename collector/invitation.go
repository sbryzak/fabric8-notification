package collector

import (
	"context"
	"fmt"

	"github.com/fabric8-services/fabric8-notification/wit/api"
	"github.com/goadesign/goa/uuid"
)

func NewInvitationResolver(witClient *api.Client) ReceiverResolver {
	return func(ctx context.Context, id string) ([]Receiver, map[string]interface{}, error) {
		userID, err := uuid.FromString(id)
		if err != nil {
			return []Receiver{}, nil, fmt.Errorf("unable to lookup user based on id %v", id)
		}
		return Invitation(ctx, witClient, userID)
	}
}

func Invitation(ctx context.Context, c *api.Client, userID uuid.UUID) ([]Receiver, map[string]interface{}, error) {

	var values = map[string]interface{}{}
	var users []uuid.UUID

	users = append(users, userID)
	//resolved, err := resolveAllUsers(ctx, witClient, SliceUniq(users), nil, true)
	//if err != nil {
//		return nil, nil, err
//	}
	//return resolved, values, nil
	return nil, values, nil
}

