// Package googlepubsub orchestrates the operations only Google Pub/Sub has.
//
// It exists beside the canonical services rather than inside them because the
// vocabulary is this family's own. A topic is declared with a retention and a
// set of labels and nothing else, where TopicService.Create collects a broker
// address, two queue counts and a permission string - RocketMQ's vocabulary,
// of which a Pub/Sub topic has none.
//
// The canonical services still serve Pub/Sub everything they can express: a
// topic is a destination and a subscription is a subscription, so listing and
// describing either goes through them. Nothing here duplicates that.
package googlepubsub

import (
	"context"
	"fmt"
	"time"

	"github.com/amigoer/mq-studio/internal/driver"
	pubsubdriver "github.com/amigoer/mq-studio/internal/driver/googlepubsub"
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

// pubsubConn resolves the connection and asserts it is this family's.
//
// The capability is checked first wherever there is one, because the reason a
// page gets back should name the capability rather than a Go type it has never
// heard of. A profile of another family reaching these methods is a bug in the
// renderer rather than an unsupported operation, and the error says so.
func (s *Service) pubsubConn(connID int, capability model.Capability) (*pubsubdriver.Conn, error) {
	conn, err := s.conns(connID)
	if err != nil {
		return nil, err
	}
	if capability != "" && !conn.Capabilities().Has(capability) {
		return nil, driver.Unsupported(conn, capability)
	}
	api, ok := conn.(*pubsubdriver.Conn)
	if !ok {
		return nil, fmt.Errorf("connection %d is %s, not google pub/sub", connID, conn.Kind())
	}
	return api, nil
}

// CreateTopic declares a topic.
//
// Beside the canonical create rather than through it, for the reason the
// package comment gives: a topic's form is a retention and a set of labels,
// and TopicService.Create has nowhere to put either.
func (s *Service) CreateTopic(ctx context.Context, connID int, spec pubsubdriver.TopicSpec) error {
	api, err := s.pubsubConn(connID, model.CapDestinationCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateTopic(ctx, spec)
}

// UpdateTopic changes an existing topic's settings.
//
// Only the fields the form sends are written: the update mask is built from
// them, so an omitted setting keeps its value rather than being reset. Labels
// are the exception and cannot be otherwise - the mask names the field, so a
// form editing them sends the complete set.
func (s *Service) UpdateTopic(ctx context.Context, connID int, spec pubsubdriver.TopicSpec) error {
	api, err := s.pubsubConn(connID, model.CapDestinationUpdate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.UpdateTopic(ctx, spec)
}

// RemoveTopic deletes a topic.
//
// Its subscriptions survive it. They keep whatever they had not delivered,
// report their topic as _deleted-topic_ from then on, and can never receive
// another message - which the subscriptions board is where to see.
func (s *Service) RemoveTopic(ctx context.Context, connID int, name string) error {
	api, err := s.pubsubConn(connID, model.CapDestinationDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveDestination(ctx, model.DestinationRef{Name: name})
}
