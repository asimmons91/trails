package channels_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/asimmons91/trails/channels"
	"github.com/stretchr/testify/require"
)

type noopChannel struct{}

func (noopChannel) Name() string { return "noop" }

func (noopChannel) Subscribed(ctx context.Context, sub *channels.Subscriber, params map[string]string) error {
	return nil
}

func (noopChannel) Unsubscribed(ctx context.Context, sub *channels.Subscriber) {}

func (noopChannel) Receive(ctx context.Context, sub *channels.Subscriber, data json.RawMessage) error {
	return nil
}

func TestRegisterPanicsOnDuplicateName(t *testing.T) {
	reg := channels.NewRegistry()
	reg.Register("chat", func() channels.Channel { return noopChannel{} })

	require.Panics(t, func() {
		reg.Register("chat", func() channels.Channel { return noopChannel{} })
	})
}
