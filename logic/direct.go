package logic

import (
	"context"

	"github.com/pkg/errors"
)

// DirectProducer hands each message straight to a ConsumerFunc instead of
// routing it through a broker.
//
// It is what the default single-process deployment uses: a broker only earns
// its keep when producers and consumers are separate processes, and running
// one locally to move messages between two goroutines costs more than it
// gives. Messages still take the same encode/decode path as they would over
// Kafka, so both modes behave identically.
type DirectProducer struct {
	ctx     context.Context
	consume ConsumerFunc
}

// NewDirectProducer returns a Producer that feeds consume synchronously.
func NewDirectProducer(ctx context.Context, consume ConsumerFunc) *DirectProducer {
	return &DirectProducer{ctx: ctx, consume: consume}
}

// Produce passes b to the consumer.
func (d *DirectProducer) Produce(b []byte) error {
	return errors.WithStack(d.consume(d.ctx, b))
}

// Close satisfies Producer. There is nothing to tear down.
func (d *DirectProducer) Close() error {
	return nil
}
