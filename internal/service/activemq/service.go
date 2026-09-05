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
	"errors"
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

// CreateSubscription registers a durable subscription on a topic.
//
// Beside the canonical create because ConsumerInput carries a broker address,
// a consume mode and a retry count - RocketMQ's vocabulary - and carries no
// topic at all. A durable subscription without the topic it reads is not a
// thing either product can make.
func (s *Service) CreateSubscription(ctx context.Context, connID int, topic, name, selector string) error {
	api, err := port[driver.SubscriptionAdmin](s, connID, model.CapSubscriptionCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateSubscription(ctx, model.SubscriptionSpec{
		Ref:        model.SubscriptionRef{Namespace: topic, Name: name},
		Attributes: map[string]string{"topic": topic, "selector": selector},
	})
}

// RemoveSubscription unsubscribes, discarding whatever it was still owed.
func (s *Service) RemoveSubscription(ctx context.Context, connID int, name string) error {
	api, err := port[driver.SubscriptionAdmin](s, connID, model.CapSubscriptionDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveSubscription(ctx, model.SubscriptionRef{Name: name})
}

// DeadLetterQueues finds the destinations dead letters land in, and what feeds
// them.
func (s *Service) DeadLetterQueues(ctx context.Context, connID int) ([]*model.DeadLetterQueue, error) {
	api, err := port[driver.DeadLetterTopology](s, connID, model.CapDeadLetterTopology)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.DeadLetterQueues(ctx, "")
}

// retrier is the driver's own retry, which is not one of the canonical ports.
//
// DeadLetterReader's ResendMessage is RocketMQ's shape - a consumer group, a
// client id and one message id - and this operation is none of those, so
// asserting the concrete surface is more honest than bending it into a port
// that means something else.
type retrier interface {
	RetryDeadLetters(ctx context.Context, ref model.DestinationRef) (int, error)
}

// RetryDeadLetters sends a dead-lettered destination's contents back to the
// destinations each message originally failed on, reporting the broker's own
// count.
func (s *Service) RetryDeadLetters(ctx context.Context, connID int, name string) (int, error) {
	api, err := port[retrier](s, connID, model.CapDeadLetterTopology)
	if err != nil {
		return 0, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RetryDeadLetters(ctx, model.DestinationRef{Name: name})
}

// publisher is the rich send, which is a canonical port rather than a
// driver-specific one - RichPublisher's shape fits well enough here.
type publisher = driver.RichPublisher

// Publish sends one or more messages to a destination.
func (s *Service) Publish(ctx context.Context, connID int, request model.PublishRequest) (*model.PublishResult, error) {
	api, err := port[publisher](s, connID, model.CapPublishRich)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.Publish(ctx, request)
}

// Connections lists what is holding a socket open on the broker.
func (s *Service) Connections(ctx context.Context, connID int) ([]*model.ClientConnection, error) {
	api, err := port[driver.ClientInspector](s, connID, model.CapClientInspect)
	if err != nil {
		if notConnected(err) {
			return nil, nil
		}
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.ListClientConnections(ctx, "")
}

// CloseConnection disconnects one client by the broker's own connection id.
func (s *Service) CloseConnection(ctx context.Context, connID int, name string) error {
	api, err := port[driver.ClientCloser](s, connID, model.CapClientClose)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CloseClientConnection(ctx, name, "")
}

// notConnected reports whether the failure is simply that nothing is dialled.
//
// List pages answer that with an empty result rather than an error: the board
// draws its own "not connected" state, and an error banner over it says the
// same thing twice.
func notConnected(err error) bool {
	return errors.Is(err, driver.ErrNotConnected)
}
