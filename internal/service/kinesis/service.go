// Package kinesis orchestrates the operations only Amazon Kinesis has.
//
// It exists beside the canonical services rather than inside them because the
// vocabulary is this family's own. A stream is created with a capacity mode, a
// shard count that only one of the two modes uses, and a retention the service
// enforces by discarding - where TopicService.Create collects a broker
// address, two queue counts and a permission string, which is RocketMQ's
// vocabulary and none of which a stream has.
//
// The canonical services still serve Kinesis everything they can express: a
// stream is a destination, and listing and describing one go through them.
// Nothing here duplicates that.
package kinesis

import (
	"context"
	"fmt"
	"time"

	"github.com/amigoer/mq-studio/internal/driver"
	kinesisdriver "github.com/amigoer/mq-studio/internal/driver/kinesis"
	"github.com/amigoer/mq-studio/internal/model"
)

// Settings exposes only what these operations need.
type Settings interface {
	GetRequestTimeout() time.Duration
}

// ConnSource yields the connection a request runs against.
type ConnSource func(connID int) (driver.Conn, error)

// Service is the orchestration layer between the bridge and the driver.
type Service struct {
	conns    ConnSource
	settings Settings
}

// New creates the service.
func New(conns ConnSource, settings Settings) *Service {
	return &Service{conns: conns, settings: settings}
}

func (s *Service) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.settings.GetRequestTimeout())
}

// kinesisConn resolves the connection and asserts it is this family's.
//
// The capability is checked first wherever there is one, because the reason a
// page gets back should name the capability rather than a Go type it has never
// heard of. A profile of another family reaching these methods is a bug in the
// renderer rather than an unsupported operation, and the error says so.
func (s *Service) kinesisConn(connID int, capability model.Capability) (*kinesisdriver.Conn, error) {
	conn, err := s.conns(connID)
	if err != nil {
		return nil, err
	}
	if capability != "" && !conn.Capabilities().Has(capability) {
		return nil, driver.Unsupported(conn, capability)
	}
	api, ok := conn.(*kinesisdriver.Conn)
	if !ok {
		return nil, fmt.Errorf("connection %d is %s, not kinesis", connID, conn.Kind())
	}
	return api, nil
}

// CreateStream declares a stream.
//
// Beside the canonical create rather than through it, for the reason the
// package comment gives: a stream's form is a capacity mode and a retention,
// and TopicService.Create has nowhere to put either.
func (s *Service) CreateStream(ctx context.Context, connID int, spec kinesisdriver.StreamSpec) error {
	api, err := s.kinesisConn(connID, model.CapDestinationCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateStream(ctx, spec)
}

// UpdateStream changes a stream's capacity mode, shard count or retention.
//
// Each of the three is a separate asynchronous operation on the service, and
// each is refused while the stream is settling from the last one - so this can
// take longer than a request that changes one thing, and the driver waits
// between them rather than firing and hoping.
func (s *Service) UpdateStream(ctx context.Context, connID int, spec kinesisdriver.StreamSpec) error {
	api, err := s.kinesisConn(connID, model.CapDestinationUpdate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.UpdateStream(ctx, spec)
}

// RemoveStream deletes a stream and every record in it. There is no undo, and
// a stream with registered consumers refuses until they are deregistered.
func (s *Service) RemoveStream(ctx context.Context, connID int, name string) error {
	api, err := s.kinesisConn(connID, model.CapDestinationDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveDestination(ctx, model.DestinationRef{Name: name})
}

/*
 * Shards lists the parts a stream is divided into, open and closed.
 *
 * Beside the canonical services because no canonical service owns this. The
 * destination service answers CapPartitions through DestinationStats, which
 * returns a read range per partition number - a shard has a name rather than a
 * number, and the fields that make it worth looking at have nowhere to go in
 * that shape.
 */
func (s *Service) Shards(ctx context.Context, connID int, stream string) ([]*model.Shard, error) {
	api, err := s.kinesisConn(connID, model.CapShards)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	// No namespace: a stream name is flat and unique within an account and
	// region, and Kinesis has nothing inside it for one to belong to.
	return api.ListShards(ctx, model.DestinationRef{Name: stream})
}

// Publish sends one body, or the same body several times, to one stream.
//
// Beside the canonical send rather than through it: MessageService.Send
// collects a topic, tags, keys and a delay level - RocketMQ's vocabulary, of
// which a Kinesis record has the destination and the key. What the canonical
// shape cannot carry is the explicit hash key, which is the only way to aim a
// record at a chosen shard.
func (s *Service) Publish(
	ctx context.Context, connID int, request kinesisdriver.PublishRequest,
) (*kinesisdriver.PublishResult, error) {
	api, err := s.kinesisConn(connID, model.CapPublish)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.Publish(ctx, request)
}

// RegisterConsumer registers an enhanced fan-out consumer on a stream.
//
// Beside the canonical create rather than through it, because the canonical
// one takes a group name and nothing else: a consumer here belongs to one
// stream, its name is unique only within that stream, and every call that
// names one takes the stream's ARN as well.
func (s *Service) RegisterConsumer(ctx context.Context, connID int, stream, name string) error {
	api, err := s.kinesisConn(connID, model.CapSubscriptionCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateSubscription(ctx, model.SubscriptionSpec{
		Ref: model.SubscriptionRef{Namespace: stream, Name: name},
	})
}

// DeregisterConsumer removes a registration.
//
// It frees the name and stops the dedicated read throughput being reserved. It
// does not stop anything reading: a classic consumer registered nothing and is
// unaffected, and an application holding a subscription is cut off at its next
// call rather than at this one.
func (s *Service) DeregisterConsumer(ctx context.Context, connID int, stream, name string) error {
	api, err := s.kinesisConn(connID, model.CapSubscriptionDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveSubscription(ctx, model.SubscriptionRef{Namespace: stream, Name: name})
}
