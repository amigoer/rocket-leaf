// Package sqs orchestrates the operations only Amazon SQS has.
//
// It exists beside the canonical services rather than inside them because the
// vocabulary is this family's own. A queue is created with a set of durations
// the service enforces and a redrive policy naming another queue, where
// TopicService.Create collects a broker address, two queue counts and a
// permission string - RocketMQ's vocabulary, of which an SQS queue has none.
//
// The canonical services still serve SQS everything they can express: a queue
// is a destination, and listing, describing and purging one all go through
// them. Nothing here duplicates that.
package sqs

import (
	"context"
	"fmt"
	"time"

	"github.com/amigoer/mq-studio/internal/driver"
	sqsdriver "github.com/amigoer/mq-studio/internal/driver/sqs"
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

// sqsConn resolves the connection and asserts it is this family's.
//
// The capability is checked first wherever there is one, because the reason a
// page gets back should name the capability rather than a Go type it has never
// heard of. A profile of another family reaching these methods is a bug in the
// renderer rather than an unsupported operation, and the error says so.
func (s *Service) sqsConn(connID int, capability model.Capability) (*sqsdriver.Conn, error) {
	conn, err := s.conns(connID)
	if err != nil {
		return nil, err
	}
	if capability != "" && !conn.Capabilities().Has(capability) {
		return nil, driver.Unsupported(conn, capability)
	}
	api, ok := conn.(*sqsdriver.Conn)
	if !ok {
		return nil, fmt.Errorf("connection %d is %s, not sqs", connID, conn.Kind())
	}
	return api, nil
}

// CreateQueue declares a queue.
//
// Beside the canonical create rather than through it, for the reason the
// package comment gives: a queue's form is a set of durations and a redrive
// policy, and TopicService.Create has nowhere to put either.
func (s *Service) CreateQueue(ctx context.Context, connID int, spec sqsdriver.QueueSpec) error {
	api, err := s.sqsConn(connID, model.CapDestinationCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateQueue(ctx, spec)
}

// UpdateQueue changes an existing queue's settings.
//
// Only the fields the form sends are written: SQS replaces exactly what it is
// given, so an omitted setting keeps its value rather than being reset.
func (s *Service) UpdateQueue(ctx context.Context, connID int, spec sqsdriver.QueueSpec) error {
	api, err := s.sqsConn(connID, model.CapDestinationUpdate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.UpdateQueue(ctx, spec)
}

// RemoveQueue deletes a queue and everything in it. There is no undo, and the
// name stays unusable for 60 seconds afterwards.
func (s *Service) RemoveQueue(ctx context.Context, connID int, name string) error {
	api, err := s.sqsConn(connID, model.CapDestinationDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveDestination(ctx, model.DestinationRef{Name: name})
}

// PurgeQueue discards everything the queue is holding.
//
// Asynchronous on the service's side: the call returning is not the queue
// being empty, and SQS allows one purge per queue per minute.
func (s *Service) PurgeQueue(ctx context.Context, connID int, name string) error {
	api, err := s.sqsConn(connID, model.CapDestinationPurge)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.PurgeQueue(ctx, model.DestinationRef{Name: name})
}

// Publish sends one body, or the same body several times, to one queue.
//
// Beside the canonical send rather than through it: MessageService.Send
// collects a topic, tags, keys and a delay level - RocketMQ's vocabulary, of
// which an SQS message has the destination and a delay in real seconds. What
// it has that the canonical shape cannot carry is a table of named attributes
// and, on a FIFO queue, the group a message is ordered within.
func (s *Service) Publish(
	ctx context.Context, connID int, request sqsdriver.PublishRequest,
) (*sqsdriver.PublishResult, error) {
	api, err := s.sqsConn(connID, model.CapPublish)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.Publish(ctx, request)
}

// DeadLetterQueues finds the queues other queues redrive into.
//
// Beside the canonical services because no canonical service owns this: the
// dead-letter page is answered three different ways across this app, and SQS's
// is the topology walk - a dead-letter queue here is an ordinary queue another
// queue's redrive policy points at.
func (s *Service) DeadLetterQueues(ctx context.Context, connID int) ([]*model.DeadLetterQueue, error) {
	api, err := s.sqsConn(connID, model.CapDeadLetterTopology)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	// No namespace: a queue name is flat and unique within an account and
	// region, and SQS has nothing inside it for one to belong to.
	return api.DeadLetterQueues(ctx, "")
}
