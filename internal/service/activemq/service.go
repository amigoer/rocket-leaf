// Package activemq orchestrates the operations only ActiveMQ has.
//
// It exists beside the canonical services rather than inside them because the
// questions are this family's own - and because one MQKind covers two products
// whose answers differ, so a page needs to know which one it is looking at.
//
// The canonical services still serve ActiveMQ everything they can express:
// queues and topics are destinations, durable subscribers are subscriptions.
// Nothing here duplicates them.
package activemq

import (
	"context"
	"time"

	"github.com/amigoer/mq-studio/internal/driver"
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

// port resolves the connection and asserts it implements what the caller
// needs, checking the declared capability first.
//
// The capability check comes before the type assertion for the reason it does
// in every other service: the reason a page gets back should name the
// capability rather than a Go type it has never heard of.
func port[T any](s *Service, connID int, capability model.Capability) (T, error) {
	var zero T
	conn, err := s.conns(connID)
	if err != nil {
		return zero, err
	}
	if !conn.Capabilities().Has(capability) {
		return zero, driver.Unsupported(conn, capability)
	}
	api, ok := conn.(T)
	if !ok {
		return zero, driver.Unsupported(conn, capability)
	}
	return api, nil
}

// PurgeQueue drops everything a destination is holding. There is no undo.
func (s *Service) PurgeQueue(ctx context.Context, connID int, ref model.DestinationRef) error {
	api, err := port[driver.QueueActions](s, connID, model.CapDestinationPurge)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.PurgeQueue(ctx, ref)
}

// MoveMessages drains one destination into another and reports how many
// arrived.
//
// The count is meaningful on an error as well as on success: what came back is
// what already reached the target, and a page that discarded it on failure
// would leave a reader unable to tell a move that did nothing from one that
// stopped halfway.
func (s *Service) MoveMessages(ctx context.Context, connID int, request model.MoveRequest) (int, error) {
	api, err := port[driver.QueueActions](s, connID, model.CapDestinationMove)
	if err != nil {
		return 0, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.MoveMessages(ctx, request)
}

// CreateDestination declares a queue or a topic.
//
// Beside the canonical create rather than through it, for the reason Kafka's
// is: TopicService.Create collects a broker address, a read queue count, a
// write queue count and a permission string, which is RocketMQ's vocabulary. A
// JMS destination has none of those and has one thing they cannot carry -
// whether it is a queue or a topic, which cannot be inferred from the name
// because both may hold the same one.
func (s *Service) CreateDestination(ctx context.Context, connID int, name string, topic bool) error {
	api, err := port[driver.DestinationAdmin](s, connID, model.CapDestinationCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	kind := "queue"
	if topic {
		kind = "topic"
	}
	return api.CreateDestination(ctx, model.DestinationSpec{
		Ref:        model.DestinationRef{Name: name},
		Attributes: map[string]string{"kind": kind},
	})
}

// RemoveDestination deletes a destination and everything it holds.
func (s *Service) RemoveDestination(ctx context.Context, connID int, name string) error {
	api, err := port[driver.DestinationAdmin](s, connID, model.CapDestinationDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveDestination(ctx, model.DestinationRef{Name: name})
}
