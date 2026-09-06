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

// CreateSubscription declares a subscription on a topic.
//
// Beside the canonical create rather than through it, for the reason the
// package comment gives: ConsumerService.Create collects a cluster, a broker
// address, a consume mode and a retry count, and a Service Bus subscription
// has none of those - what it has is the same delivery contract a queue has,
// because on this family a subscription is where the messages are.
func (s *Service) CreateSubscription(
	ctx context.Context, connID int, spec servicebusdriver.SubscriptionSpec,
) error {
	api, err := s.serviceBusConn(connID, model.CapSubscriptionCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateSubscriptionFrom(ctx, spec)
}

// UpdateSubscription changes what a subscription lets be changed. The topic
// and sessions are fixed at creation and are not among them.
func (s *Service) UpdateSubscription(
	ctx context.Context, connID int, spec servicebusdriver.SubscriptionSpec,
) error {
	api, err := s.serviceBusConn(connID, model.CapSubscriptionCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.UpdateSubscriptionFrom(ctx, spec)
}

// RemoveSubscription deletes a subscription and everything it was holding.
//
// Its dead letters go with it, and nothing is handed back to the topic: a copy
// that reached this subscription was never the topic's again.
func (s *Service) RemoveSubscription(ctx context.Context, connID int, topic, name string) error {
	api, err := s.serviceBusConn(connID, model.CapSubscriptionDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveSubscription(ctx, model.SubscriptionRef{Namespace: topic, Name: name})
}

// Send publishes one message, or the same one several times, to one entity.
//
// Beside the canonical send rather than through it: MessageService.Send
// collects a topic, tags, keys and a delay level - RocketMQ's vocabulary, of
// which a Service Bus message has the body and little else. What it has that
// the canonical shape cannot carry is a table of named properties and a
// subject, which together are what a subscription's rules select on.
func (s *Service) Send(
	ctx context.Context, connID int, request servicebusdriver.SendRequest,
) (*servicebusdriver.SendResult, error) {
	api, err := s.serviceBusConn(connID, model.CapPublish)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.Send(ctx, request)
}

// CancelScheduled takes back messages that have not been enqueued yet.
//
// Gated on the delay capability rather than on the send, because that is what
// produced the sequence numbers it takes: without scheduling there is nothing
// to cancel.
func (s *Service) CancelScheduled(
	ctx context.Context, connID int, entity string, sequences []int64,
) error {
	api, err := s.serviceBusConn(connID, model.CapDelayedDelivery)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CancelScheduled(ctx, entity, sequences)
}

// CreateRule declares a rule on a subscription.
//
// Beside the canonical routing service because that one only reads: a rule is
// created and deleted through the family's own vocabulary, and RoutingMutator
// speaks in bindings with a source, a destination and a routing key, which is
// not what a form filling in a SQL filter or a correlation filter collects.
func (s *Service) CreateRule(ctx context.Context, connID int, spec servicebusdriver.RuleSpec) error {
	api, err := s.serviceBusConn(connID, model.CapRoutingAdmin)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateRule(ctx, spec)
}

// RemoveRule deletes one rule.
//
// Worth knowing before the last one goes: a subscription with no rules
// receives nothing at all, while reporting itself Active with an empty
// backlog. The subscriptions board reports that state as offline rather than
// letting it look healthy.
func (s *Service) RemoveRule(ctx context.Context, connID int, topic, subscription, name string) error {
	api, err := s.serviceBusConn(connID, model.CapRoutingAdmin)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveRule(ctx, topic, subscription, name)
}
