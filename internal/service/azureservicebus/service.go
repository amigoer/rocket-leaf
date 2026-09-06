// Package azureservicebus orchestrates the operations only Azure Service Bus
// has.
//
// It exists beside the canonical services rather than inside them because the
// vocabulary is this family's own. An entity is declared with a lock duration,
// a delivery limit and somewhere to forward what it gives up on, where
// TopicService.Create collects a broker address, two queue counts and a
// permission string - RocketMQ's vocabulary, of which a Service Bus queue has
// none.
//
// The canonical services still serve Service Bus everything they can express:
// a queue and a topic are destinations and a subscription is a subscription,
// so listing and describing any of them goes through them. Nothing here
// duplicates that.
package azureservicebus

import (
	"context"
	"fmt"
	"time"

	"github.com/amigoer/mq-studio/internal/driver"
	servicebusdriver "github.com/amigoer/mq-studio/internal/driver/azureservicebus"
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

// serviceBusConn resolves the connection and asserts it is this family's.
//
// The capability is checked first wherever there is one, because the reason a
// page gets back should name the capability rather than a Go type it has never
// heard of. A profile of another family reaching these methods is a bug in the
// renderer rather than an unsupported operation, and the error says so.
func (s *Service) serviceBusConn(connID int, capability model.Capability) (*servicebusdriver.Conn, error) {
	conn, err := s.conns(connID)
	if err != nil {
		return nil, err
	}
	if capability != "" && !conn.Capabilities().Has(capability) {
		return nil, driver.Unsupported(conn, capability)
	}
	api, ok := conn.(*servicebusdriver.Conn)
	if !ok {
		return nil, fmt.Errorf("connection %d is %s, not azure service bus", connID, conn.Kind())
	}
	return api, nil
}

// CreateEntity declares a queue or a topic.
//
// Beside the canonical create rather than through it, for the reason the
// package comment gives: an entity's form is a delivery contract, and
// TopicService.Create has nowhere to put a lock duration or a delivery limit.
func (s *Service) CreateEntity(ctx context.Context, connID int, spec servicebusdriver.EntitySpec) error {
	api, err := s.serviceBusConn(connID, model.CapDestinationCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateEntity(ctx, spec)
}

// UpdateEntity changes an existing entity's settings.
//
// Three of them cannot change and are not sent: sessions, duplicate detection
// and partitioning are fixed at creation. The kind cannot change either, and
// the driver refuses rather than replacing the entity with the other sort.
func (s *Service) UpdateEntity(ctx context.Context, connID int, spec servicebusdriver.EntitySpec) error {
	api, err := s.serviceBusConn(connID, model.CapDestinationUpdate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.UpdateEntity(ctx, spec)
}

// RemoveEntity deletes a queue or a topic.
//
// Everything it holds goes with it, dead letters included: a $DeadLetterQueue
// is a sub-entity rather than a queue that survives its parent. Deleting a
// topic takes every subscription on it, and with them whatever those had not
// delivered.
func (s *Service) RemoveEntity(ctx context.Context, connID int, name string) error {
	api, err := s.serviceBusConn(connID, model.CapDestinationDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveDestination(ctx, model.DestinationRef{Name: name})
}
