package bridge

import (
	"context"

	pubsubdriver "github.com/amigoer/mq-studio/internal/driver/googlepubsub"
	pubsubservice "github.com/amigoer/mq-studio/internal/service/googlepubsub"
)

// GooglePubSubService is the renderer's entry point for the operations only
// Google Pub/Sub has. Listing and describing topics go through the canonical
// services; what is here is the rest.
type GooglePubSubService struct {
	service *pubsubservice.Service
}

// NewGooglePubSubService wires the bridge to the service.
func NewGooglePubSubService(service *pubsubservice.Service) *GooglePubSubService {
	return &GooglePubSubService{service: service}
}

// GooglePubSubTopicInput is a topic as the topic form collects it.
//
// Deliberately not TopicService.Create's shape. That one takes a broker
// address, two queue counts and a permission string - RocketMQ's vocabulary,
// of which a Pub/Sub topic has none. What a topic has instead is almost
// nothing, because everything about delivery belongs to the subscription.
type GooglePubSubTopicInput struct {
	Name string `json:"name"`

	// RetentionSec keeps published messages available for a subscription to
	// seek back into. Zero leaves it alone, which on an edit means "keep what
	// is stored" and on a create takes the service's default.
	RetentionSec int `json:"retentionSec"`

	// Labels replace the topic's whole set rather than merging into it, which
	// is the API's own behaviour: the update mask names the field, not one key.
	Labels map[string]string `json:"labels"`
}

func (input GooglePubSubTopicInput) spec() pubsubdriver.TopicSpec {
	return pubsubdriver.TopicSpec{
		Name:         input.Name,
		RetentionSec: input.RetentionSec,
		Labels:       input.Labels,
	}
}

// CreateTopic declares a topic in the connection's project.
func (s *GooglePubSubService) CreateTopic(connID int, input GooglePubSubTopicInput) error {
	return s.service.CreateTopic(context.Background(), connID, input.spec())
}

// UpdateTopic changes an existing topic's settings. Labels are replaced
// wholesale, so a form editing them sends the complete set.
func (s *GooglePubSubService) UpdateTopic(connID int, input GooglePubSubTopicInput) error {
	return s.service.UpdateTopic(context.Background(), connID, input.spec())
}

// RemoveTopic deletes a topic. Its subscriptions survive it, pointing at
// _deleted-topic_ and unable to receive anything ever again.
func (s *GooglePubSubService) RemoveTopic(connID int, name string) error {
	return s.service.RemoveTopic(context.Background(), connID, name)
}

// GooglePubSubSubscriptionInput is a subscription as its form collects it.
//
// Deliberately not ConsumerService.Create's shape. That one takes a cluster, a
// broker address, a consume mode and a retry count - RocketMQ's vocabulary, of
// which a Pub/Sub subscription has none. What it has instead is the whole of
// the delivery configuration, because on this family that belongs to the
// subscription rather than to the topic it reads.
//
// Every duration is zero when the form left it alone, which on an edit means
// "keep what is stored": the update mask is built from what is set, so an
// omitted setting survives.
type GooglePubSubSubscriptionInput struct {
	Name string `json:"name"`
	// Topic is required on a create and ignored on an update: a subscription
	// reads exactly one topic, chosen when it is made.
	Topic string `json:"topic"`

	AckDeadlineSec int  `json:"ackDeadlineSec"`
	RetentionSec   int  `json:"retentionSec"`
	RetainAcked    bool `json:"retainAcked"`
	ExactlyOnce    bool `json:"exactlyOnce"`

	// Filter and Ordering are fixed at creation. They are sent on an update
	// too and ignored there, so a form can round-trip one object.
	Filter   string `json:"filter"`
	Ordering bool   `json:"ordering"`

	// PushEndpoint makes this a push subscription, which Pub/Sub POSTs to
	// rather than holding for a reader.
	PushEndpoint string `json:"pushEndpoint"`

	// DeadLetterTopic is another topic's name; the driver resolves its path.
	// Empty on an update removes the policy, which is the only way to.
	DeadLetterTopic string `json:"deadLetterTopic"`
	MaxAttempts     int    `json:"maxAttempts"`

	RetryMinBackoffSec int `json:"retryMinBackoffSec"`
	RetryMaxBackoffSec int `json:"retryMaxBackoffSec"`

	// Labels are set at creation only: the emulator refuses them in an update
	// mask, and a control that fails against the reference environment is
	// worse than one that is not offered.
	Labels map[string]string `json:"labels"`
}

func (input GooglePubSubSubscriptionInput) spec() pubsubdriver.SubscriptionSpec {
	return pubsubdriver.SubscriptionSpec{
		Name:               input.Name,
		Topic:              input.Topic,
		AckDeadlineSec:     input.AckDeadlineSec,
		RetentionSec:       input.RetentionSec,
		RetainAcked:        input.RetainAcked,
		ExactlyOnce:        input.ExactlyOnce,
		Filter:             input.Filter,
		Ordering:           input.Ordering,
		PushEndpoint:       input.PushEndpoint,
		DeadLetterTopic:    input.DeadLetterTopic,
		MaxAttempts:        input.MaxAttempts,
		RetryMinBackoffSec: input.RetryMinBackoffSec,
		RetryMaxBackoffSec: input.RetryMaxBackoffSec,
		Labels:             input.Labels,
	}
}

// CreateSubscription declares a subscription on a topic.
func (s *GooglePubSubService) CreateSubscription(
	connID int, input GooglePubSubSubscriptionInput,
) error {
	return s.service.CreateSubscription(context.Background(), connID, input.spec())
}

// UpdateSubscription changes what a subscription lets be changed. The topic,
// the filter and message ordering are fixed at creation and are not among them.
func (s *GooglePubSubService) UpdateSubscription(
	connID int, input GooglePubSubSubscriptionInput,
) error {
	return s.service.UpdateSubscription(context.Background(), connID, input.spec())
}

// RemoveSubscription deletes a subscription and everything it had not
// acknowledged. Those messages were never the topic's to hand out again.
func (s *GooglePubSubService) RemoveSubscription(connID int, name string) error {
	return s.service.RemoveSubscription(context.Background(), connID, name)
}

// GooglePubSubSnapshot is a restore point taken from one subscription.
type GooglePubSubSnapshot struct {
	Name string `json:"name"`
	// Topic is what the subscription it was taken from reads. A snapshot can
	// only be sought to from a subscription on the same topic.
	Topic string `json:"topic"`
	// ExpiresAtMs is when the service will delete it. A snapshot lives seven
	// days, and until then the topic keeps everything it could restore.
	ExpiresAtMs int64 `json:"expiresAtMs"`
}

// ListSnapshots is every restore point in the project.
func (s *GooglePubSubService) ListSnapshots(connID int) ([]GooglePubSubSnapshot, error) {
	found, err := s.service.ListSnapshots(context.Background(), connID)
	if err != nil {
		return nil, err
	}
	snapshots := make([]GooglePubSubSnapshot, 0, len(found))
	for _, snapshot := range found {
		snapshots = append(snapshots, GooglePubSubSnapshot{
			Name:        snapshot.Name,
			Topic:       snapshot.Topic,
			ExpiresAtMs: snapshot.ExpiresAt,
		})
	}
	return snapshots, nil
}

// CreateSnapshot takes a restore point from one subscription.
func (s *GooglePubSubService) CreateSnapshot(connID int, name, subscription string) error {
	return s.service.CreateSnapshot(context.Background(), connID, name, subscription)
}

// RemoveSnapshot deletes a restore point, and with it the reason the topic was
// holding everything it could restore.
func (s *GooglePubSubService) RemoveSnapshot(connID int, name string) error {
	return s.service.RemoveSnapshot(context.Background(), connID, name)
}

// SeekToSnapshot moves a subscription to a restore point.
//
// The other half of Seek - moving to a moment in time - goes through the
// canonical consumer service, and is degraded against an emulator because the
// emulator answers it Unimplemented.
func (s *GooglePubSubService) SeekToSnapshot(connID int, subscription, snapshot string) error {
	return s.service.SeekToSnapshot(context.Background(), connID, subscription, snapshot)
}
