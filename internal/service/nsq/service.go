// Package nsq orchestrates the operations only NSQ has.
//
// It exists beside the canonical services rather than inside them because the
// gestures are this family's own. Pausing is the clearest case: it is neither
// a purge nor a delete, publishing carries on while it is in force, and no
// canonical port describes it - so there is no Capability that could gate it
// and the gate is the driver itself.
//
// The canonical services still serve NSQ everything they can express: topics
// are destinations and channels are subscriptions. Nothing here duplicates
// them.
package nsq

import (
	"context"
	"fmt"
	"time"

	"github.com/amigoer/mq-studio/internal/driver"
	nsqdriver "github.com/amigoer/mq-studio/internal/driver/nsq"
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

// nsqConn resolves the connection and asserts it is this family's.
//
// There is no capability to gate on: pausing has no canonical port and
// therefore no Capability that could describe it, so the gate is the driver
// itself. A profile of another family reaching these methods is a bug in the
// renderer rather than an unsupported operation, and the error says so.
func (s *Service) nsqConn(connID int) (*nsqdriver.Conn, error) {
	conn, err := s.conns(connID)
	if err != nil {
		return nil, err
	}
	api, ok := conn.(*nsqdriver.Conn)
	if !ok {
		return nil, fmt.Errorf("connection %d is %s, not nsq", connID, conn.Kind())
	}
	return api, nil
}

// CreateTopic declares a topic on every nsqd in the connection.
//
// Beside the canonical create rather than through it, for the reason
// ActiveMQ's is: TopicService.Create collects a broker address, a read queue
// count, a write queue count and a permission string, which is RocketMQ's
// vocabulary. An NSQ topic has a name and nothing else.
func (s *Service) CreateTopic(ctx context.Context, connID int, name string) error {
	api, err := port[driver.DestinationAdmin](s, connID, model.CapDestinationCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateDestination(ctx, model.DestinationSpec{Ref: model.DestinationRef{Name: name}})
}

// RemoveTopic deletes a topic, its channels, and its registration in the
// discovery tier. There is no undo.
func (s *Service) RemoveTopic(ctx context.Context, connID int, name string) error {
	api, err := port[driver.DestinationAdmin](s, connID, model.CapDestinationDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveDestination(ctx, model.DestinationRef{Name: name})
}

// EmptyTopic discards everything the topic and its channels are holding.
func (s *Service) EmptyTopic(ctx context.Context, connID int, name string) error {
	api, err := port[driver.QueueActions](s, connID, model.CapDestinationPurge)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.PurgeQueue(ctx, model.DestinationRef{Name: name})
}

// SetTopicPaused stops or resumes delivery into a topic's channels.
func (s *Service) SetTopicPaused(ctx context.Context, connID int, name string, paused bool) error {
	api, err := s.nsqConn(connID)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.SetTopicPaused(ctx, name, paused)
}

// CreateChannel declares a channel on every nsqd carrying its topic.
//
// Beside the canonical create rather than through it: ConsumerService.Create
// collects a RocketMQ consumer group - a broker address, a retry queue count,
// a consume-from-where - and an NSQ channel has a topic and a name.
func (s *Service) CreateChannel(ctx context.Context, connID int, topic, channel string) error {
	api, err := port[driver.SubscriptionAdmin](s, connID, model.CapSubscriptionCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateSubscription(ctx, model.SubscriptionSpec{
		Ref: model.SubscriptionRef{Namespace: topic, Name: channel},
	})
}

// RemoveChannel deletes a channel and its backlog. There is no undo.
func (s *Service) RemoveChannel(ctx context.Context, connID int, topic, channel string) error {
	api, err := port[driver.SubscriptionAdmin](s, connID, model.CapSubscriptionDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveSubscription(ctx, model.SubscriptionRef{Namespace: topic, Name: channel})
}

// EmptyChannel discards one channel's backlog, leaving the topic and the other
// channels under it alone.
func (s *Service) EmptyChannel(ctx context.Context, connID int, topic, channel string) error {
	api, err := s.nsqConn(connID)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.EmptyChannel(ctx, topic, channel)
}

// SetChannelPaused stops or resumes delivery to one channel's consumers.
func (s *Service) SetChannelPaused(
	ctx context.Context, connID int, topic, channel string, paused bool,
) error {
	api, err := s.nsqConn(connID)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.SetChannelPaused(ctx, topic, channel, paused)
}
