package cloudtask

import (
	"context"

	"google.golang.org/api/idtoken"
)

type TokenVerifier interface {
	Validate(ctx context.Context, idToken, audience string) (*idtoken.Payload, error)
}

type defaultVerifier struct{}

func (defaultVerifier) Validate(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
	return idtoken.Validate(ctx, idToken, audience)
}

var _ TokenVerifier = defaultVerifier{}
