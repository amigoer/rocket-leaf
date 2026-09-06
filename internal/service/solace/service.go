// Package solace orchestrates the operations only Solace has.
//
// It exists beside the canonical services rather than inside them because the
// vocabulary is this family's own. Creating a queue here means choosing an
// access type - whether one consumer takes everything or several share it -
// and a permission for whatever binds to it, where TopicService.Create
// collects a broker address, two queue counts and a permission string, which
// is RocketMQ's vocabulary and none of which a Solace queue has.
//
// The canonical services still serve Solace everything they can express:
// listing and describing a queue goes through them. Nothing here duplicates
// that.
package solace

import (
	"context"
	"fmt"
	"time"

	"github.com/amigoer/mq-studio/internal/driver"
	solacedriver "github.com/amigoer/mq-studio/internal/driver/solace"
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

// solaceConn resolves the connection and asserts it is this family's.
//
// The capability is checked first wherever there is one, because the reason a
// page gets back should name the capability rather than a Go type it has never
// heard of. A profile of another family reaching these methods is a bug in the
// renderer rather than an unsupported operation, and the error says so.
func (s *Service) solaceConn(connID int, capability model.Capability) (*solacedriver.Conn, error) {
	conn, err := s.conns(connID)
	if err != nil {
		return nil, err
	}
	if capability != "" && !conn.Capabilities().Has(capability) {
		return nil, driver.Unsupported(conn, capability)
	}
	api, ok := conn.(*solacedriver.Conn)
	if !ok {
		return nil, fmt.Errorf("connection %d is %s, not solace", connID, conn.Kind())
	}
	return api, nil
}

// MsgVPN is which Message VPN this connection reads. Every board prints it,
// because the sidebar can re-point the connection at another one and nothing
// else on a board would say which it is on.
func (s *Service) MsgVPN(connID int) (string, error) {
	api, err := s.solaceConn(connID, "")
	if err != nil {
		return "", err
	}
	return api.MsgVPN(), nil
}

// CreateDestination declares a queue in this Message VPN.
//
// Beside the canonical create rather than through it, for the reason the
// package comment gives: the access type and the permission decide how the
// queue behaves for every consumer that ever binds to it, and the canonical
// spec has nowhere to say either.
func (s *Service) CreateDestination(ctx context.Context, connID int, spec model.DestinationSpec) error {
	api, err := s.solaceConn(connID, model.CapDestinationCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateDestination(ctx, spec)
}

/*
 * RemoveDestination deletes a queue, and whatever it was holding.
 *
 * No purge flag, unlike IBM MQ's, and its absence is the broker's rather than
 * an omission: SEMP deletes a queue holding a quarter of a million messages as
 * readily as an empty one and offers no precondition to ask for. A flag here
 * would be this app inventing a guard and presenting it as the broker's, so
 * what the caller gets instead is the count on the row and a confirmation that
 * says the messages go too.
 */
func (s *Service) RemoveDestination(ctx context.Context, connID int, name string) error {
	api, err := s.solaceConn(connID, model.CapDestinationDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	// The namespace is left empty: the driver builds every path from the
	// Message VPN the connection settled on, and a ref naming another one
	// would be asking it to write outside its own scope.
	return api.RemoveDestination(ctx, model.DestinationRef{Name: name})
}

/*
 * Publish sends one body, or the same body several times.
 *
 * Beside the canonical send rather than through it: MessageService.Send
 * collects a topic, tags, keys and a delay level - RocketMQ's vocabulary, of
 * which a Solace message has only the destination. What it carries instead is
 * a delivery mode that decides whether the broker spools it at all, a time to
 * live, and the flag that decides whether it is moved or discarded when it is
 * given up on.
 *
 * It also goes somewhere else entirely: SEMP carries no message data, so this
 * is the REST messaging interface on its own port, probed when the connection
 * opened. CapPublish is what says the probe succeeded.
 */
func (s *Service) Publish(
	ctx context.Context, connID int, request solacedriver.PublishRequest,
) (*solacedriver.PublishResult, error) {
	api, err := s.solaceConn(connID, model.CapPublish)
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
 * Beside the canonical services because no canonical service owns this shape:
 * the destination service lists every queue, and which of them is a dead
 * message queue is a fact about what points at it rather than about the queue
 * itself. It is also the only page that reports a target which does not exist,
 * which no listing could.
 */
func (s *Service) DeadLetters(ctx context.Context, connID int) ([]*model.DeadLetterQueue, error) {
	api, err := s.solaceConn(connID, model.CapDeadLetterTopology)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.DeadLetterQueues(ctx, "")
}
