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

// CreateSubscription declares a subscription on a topic.
//
// Beside the canonical create rather than through it, for the reason the
// package comment gives: ConsumerService.Create collects a cluster, a broker
// address, a consume mode and a retry count, and a Pub/Sub subscription has
// none of those - what it has is the whole of the delivery configuration,
// because on this family that belongs to the subscription.
func (s *Service) CreateSubscription(
	ctx context.Context, connID int, spec pubsubdriver.SubscriptionSpec,
) error {
	api, err := s.pubsubConn(connID, model.CapSubscriptionCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateSubscriptionFrom(ctx, spec)
}

// UpdateSubscription changes what a subscription lets be changed. The topic,
// the filter and message ordering are fixed at creation and are not among them.
func (s *Service) UpdateSubscription(
	ctx context.Context, connID int, spec pubsubdriver.SubscriptionSpec,
) error {
	api, err := s.pubsubConn(connID, model.CapSubscriptionCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.UpdateSubscriptionFrom(ctx, spec)
}

// RemoveSubscription deletes a subscription and everything it was holding.
//
// The messages it had not acknowledged go with it. They were never the topic's
// to hand out again, which is the whole point of the split between the two.
func (s *Service) RemoveSubscription(ctx context.Context, connID int, name string) error {
	api, err := s.pubsubConn(connID, model.CapSubscriptionDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveSubscription(ctx, model.SubscriptionRef{Name: name})
}

// ListSnapshots is every restore point in the project.
//
// Gated on the position capability rather than a listing one, because that is
// what a snapshot is for here: it is the only place a subscription can be
// sought to that the emulator serves, so without it the position control has
// nothing to point at.
func (s *Service) ListSnapshots(ctx context.Context, connID int) ([]*pubsubdriver.Snapshot, error) {
	api, err := s.pubsubConn(connID, model.CapSubscriptionPosition)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.ListSnapshots(ctx)
}

// CreateSnapshot takes a restore point from one subscription.
//
// It belongs to the topic rather than to the subscription it came from: any
// subscription on the same topic can be sought to it, and the topic keeps
// every message the snapshot could restore until it is deleted or expires.
func (s *Service) CreateSnapshot(ctx context.Context, connID int, name, subscription string) error {
	api, err := s.pubsubConn(connID, model.CapSubscriptionPosition)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateSnapshot(ctx, name, subscription)
}

// RemoveSnapshot deletes a restore point, and with it the reason the topic was
// holding what it could restore.
func (s *Service) RemoveSnapshot(ctx context.Context, connID int, name string) error {
	api, err := s.pubsubConn(connID, model.CapSubscriptionPosition)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveSnapshot(ctx, name)
}

// SeekToSnapshot moves a subscription to a restore point.
//
// The other half of Seek - moving to a moment in time - goes through the
// canonical ConsumerService.ResetOffset, and is degraded against an emulator
// because the emulator answers it Unimplemented.
func (s *Service) SeekToSnapshot(ctx context.Context, connID int, subscription, snapshot string) error {
	api, err := s.pubsubConn(connID, model.CapSubscriptionPosition)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.SetSubscriptionPosition(ctx, model.PositionRequest{
		Ref:      model.SubscriptionRef{Name: subscription},
		Position: snapshot,
	})
}

// Publish sends one body, or the same body several times, to one topic.
//
// Beside the canonical send rather than through it: MessageService.Send
// collects a topic, tags, keys and a delay level - RocketMQ's vocabulary, of
// which a Pub/Sub message has only the topic. What it has that the canonical
// shape cannot carry is a table of named attributes, which is also the only
// thing a subscription filter can select on, and an ordering key.
func (s *Service) Publish(
	ctx context.Context, connID int, request pubsubdriver.PublishRequest,
) (*pubsubdriver.PublishResult, error) {
	api, err := s.pubsubConn(connID, model.CapPublish)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.Publish(ctx, request)
}
