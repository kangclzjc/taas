package natsutil

import (
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
)

// SubscribePushDurable creates a JetStream push subscription with a durable name.
// If the server returns a stale "already bound" bind error (e.g. after gateway restarts),
// it deletes that durable consumer once and retries the subscribe.
func SubscribePushDurable(
	js nats.JetStreamContext,
	subject string,
	cb nats.MsgHandler,
	stream, durable string,
	opts ...nats.SubOpt,
) (*nats.Subscription, error) {
	all := append([]nats.SubOpt{nats.Durable(durable)}, opts...)
	sub, err := js.Subscribe(subject, cb, all...)
	if err == nil {
		return sub, nil
	}
	if !isStaleConsumerBindError(err) {
		return nil, err
	}
	if delErr := js.DeleteConsumer(stream, durable); delErr != nil {
		return nil, fmt.Errorf("subscribe: %w; delete consumer %q: %w", err, durable, delErr)
	}
	sub, err = js.Subscribe(subject, cb, all...)
	if err != nil {
		return nil, fmt.Errorf("subscribe after consumer recreate: %w", err)
	}
	return sub, nil
}

func isStaleConsumerBindError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "already bound") || strings.Contains(s, "consumer is already bound")
}
