package cloudtask

import (
	"context"

	"google.golang.org/api/idtoken"
)

// TokenVerifier verifies the OIDC bearer token Cloud Tasks attaches to a
// push request. The default implementation wraps idtoken.Validate;
// WithTokenVerifier lets tests or alternate deployments substitute another.
type TokenVerifier interface {
	// Validate verifies idToken was issued for audience and returns its
	// claims.
	Validate(ctx context.Context, idToken, audience string) (*idtoken.Payload, error)
}

type defaultVerifier struct{}

func (defaultVerifier) Validate(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
	return idtoken.Validate(ctx, idToken, audience)
}

var _ TokenVerifier = defaultVerifier{}
