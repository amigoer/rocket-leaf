// Package ibmmq orchestrates the operations only IBM MQ has.
//
// It exists beside the canonical services rather than inside them because the
// vocabulary is this family's own. A destination here is a queue or a topic,
// which are different objects with different fields - a queue has a maximum
// depth and a type, a topic has a topic string publishers name - where
// TopicService.Create collects a broker address, two queue counts and a
// permission string, which is RocketMQ's vocabulary and none of which either
// object has.
//
// The canonical services still serve IBM MQ everything they can express: a
// queue and a topic are both destinations, and listing and describing one goes
// through them. Nothing here duplicates that.
package ibmmq

import (
	"context"
	"fmt"
	"time"

	"github.com/amigoer/mq-studio/internal/driver"
	ibmmqdriver "github.com/amigoer/mq-studio/internal/driver/ibmmq"
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

// ibmmqConn resolves the connection and asserts it is this family's.
//
// The capability is checked first wherever there is one, because the reason a
// page gets back should name the capability rather than a Go type it has never
// heard of. A profile of another family reaching these methods is a bug in the
// renderer rather than an unsupported operation, and the error says so.
func (s *Service) ibmmqConn(connID int, capability model.Capability) (*ibmmqdriver.Conn, error) {
	conn, err := s.conns(connID)
	if err != nil {
		return nil, err
	}
	if capability != "" && !conn.Capabilities().Has(capability) {
		return nil, driver.Unsupported(conn, capability)
	}
	api, ok := conn.(*ibmmqdriver.Conn)
	if !ok {
		return nil, fmt.Errorf("connection %d is %s, not ibm mq", connID, conn.Kind())
	}
	return api, nil
}

// QueueManager is which queue manager this connection speaks to. Every board
// prints it, because a second queue manager behind the same server is a
// different set of objects entirely.
func (s *Service) QueueManager(connID int) (string, error) {
	api, err := s.ibmmqConn(connID, "")
	if err != nil {
		return "", err
	}
	return api.QueueManager(), nil
}

// CreateDestination declares a queue or a topic.
//
// Beside the canonical create rather than through it, for the reason the
// package comment gives: which of the two is being made decides the whole
// shape of the request, and the canonical spec has nowhere to say so.
func (s *Service) CreateDestination(ctx context.Context, connID int, spec model.DestinationSpec) error {
	api, err := s.ibmmqConn(connID, model.CapDestinationCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateDestination(ctx, spec)
}

/*
 * RemoveDestination deletes a queue or a topic.
 *
 * purge is the whole of the decision. Without it the queue manager refuses a
 * queue that holds messages, which is the check worth keeping as the default;
 * with it the messages go with the queue and there is no undo. A queue an
 * application has open is refused either way, by the queue manager, and no
 * flag overrides that.
 */
func (s *Service) RemoveDestination(ctx context.Context, connID int, name string, purge bool) error {
	api, err := s.ibmmqConn(connID, model.CapDestinationDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	// No namespace: a queue or topic name is flat and unique within its queue
	// manager, and MQ has nothing inside one for a name to belong to.
	ref := model.DestinationRef{Name: name}
	if purge {
		return api.RemoveQueueGuarded(ctx, ref, true, false)
	}
	return api.RemoveDestination(ctx, ref)
}

/*
 * Channels lists the objects clients and peers reach this queue manager
 * through.
 *
 * Beside the canonical services because no canonical service owns this. The
 * closest is the client service, which answers CapClientInspect with the
 * transport connections open right now - a channel is the definition those
 * connections have to come through, it exists with nothing connected, and one
 * definition carries many connections at once.
 */
func (s *Service) Channels(ctx context.Context, connID int) ([]*model.Channel, error) {
	api, err := s.ibmmqConn(connID, model.CapChannels)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.ListChannels(ctx)
}

// Publish sends one body, or the same body several times, to one queue.
//
// Beside the canonical send rather than through it: MessageService.Send
// collects a topic, tags, keys and a delay level - RocketMQ's vocabulary, of
// which an MQ message has none. What it carries instead is a descriptor, and
// what the canonical shape cannot express is the whole of it.
func (s *Service) Publish(
	ctx context.Context, connID int, request ibmmqdriver.PublishRequest,
) (*ibmmqdriver.PublishResult, error) {
	api, err := s.ibmmqConn(connID, model.CapPublish)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.Publish(ctx, request)
}

/*
 * DeadLetters finds the queues something else dead-letters into.
 *
 * Beside the canonical services because no canonical service owns this
 * shape: the destination service lists every queue, and which of them is a
 * dead-letter queue is a fact about what points at it rather than about the
 * queue itself.
 */
func (s *Service) DeadLetters(ctx context.Context, connID int) ([]*model.DeadLetterQueue, error) {
	api, err := s.ibmmqConn(connID, model.CapDeadLetterTopology)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.DeadLetterQueues(ctx, "")
}
