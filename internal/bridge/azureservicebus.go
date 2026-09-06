package bridge

import (
	"context"

	servicebusdriver "github.com/amigoer/mq-studio/internal/driver/azureservicebus"
	servicebusservice "github.com/amigoer/mq-studio/internal/service/azureservicebus"
)

// AzureServiceBusService is the renderer's entry point for the operations only
// Azure Service Bus has. Listing and describing entities go through the
// canonical services; what is here is the rest.
type AzureServiceBusService struct {
	service *servicebusservice.Service
}

// NewAzureServiceBusService wires the bridge to the service.
func NewAzureServiceBusService(service *servicebusservice.Service) *AzureServiceBusService {
	return &AzureServiceBusService{service: service}
}

// AzureServiceBusEntityInput is a queue or a topic as its form collects it.
//
// Deliberately not TopicService.Create's shape. That one takes a broker
// address, two queue counts and a permission string - RocketMQ's vocabulary,
// of which a Service Bus entity has none. What it has instead is a delivery
// contract: how long a receiver holds a message, how many times it may be
// tried, how long it lives, and where it goes when it is given up on.
//
// Every duration is zero when the form left it alone, which on an edit means
// "keep what is stored".
type AzureServiceBusEntityInput struct {
	Name string `json:"name"`
	// Kind is "queue" or "topic" and is fixed once the entity exists: the two
	// are different objects, so changing it would mean deleting one and
	// losing whatever it held. The driver refuses rather than doing that.
	Kind string `json:"kind"`

	// The delivery half, which belongs to a queue. A topic carries none of it:
	// its subscriptions do, because that is where the messages end up.
	LockDurationSec    int  `json:"lockDurationSec"`
	MaxDeliveryCount   int  `json:"maxDeliveryCount"`
	DeadLetterOnExpiry bool `json:"deadLetterOnExpiry"`

	TTLSec              int `json:"ttlSec"`
	AutoDeleteOnIdleSec int `json:"autoDeleteOnIdleSec"`
	MaxSizeMB           int `json:"maxSizeMb"`

	// Fixed at creation: the service refuses all three in an update.
	RequiresSession            bool `json:"requiresSession"`
	RequiresDuplicateDetection bool `json:"requiresDuplicateDetection"`
	Partitioned                bool `json:"partitioned"`

	// ForwardTo hands every arriving message to another entity, and
	// ForwardDeadLettersTo does the same for what this one gives up on. Empty
	// removes the forwarding, which is the only way to.
	ForwardTo            string `json:"forwardTo"`
	ForwardDeadLettersTo string `json:"forwardDeadLettersTo"`
}

func (input AzureServiceBusEntityInput) spec() servicebusdriver.EntitySpec {
	return servicebusdriver.EntitySpec{
		Name:                       input.Name,
		Kind:                       input.Kind,
		LockDurationSec:            input.LockDurationSec,
		MaxDeliveryCount:           input.MaxDeliveryCount,
		DeadLetterOnExpiry:         input.DeadLetterOnExpiry,
		TTLSec:                     input.TTLSec,
		AutoDeleteOnIdleSec:        input.AutoDeleteOnIdleSec,
		MaxSizeMB:                  input.MaxSizeMB,
		RequiresSession:            input.RequiresSession,
		RequiresDuplicateDetection: input.RequiresDuplicateDetection,
		Partitioned:                input.Partitioned,
		ForwardTo:                  input.ForwardTo,
		ForwardDeadLettersTo:       input.ForwardDeadLettersTo,
	}
}

// CreateEntity declares a queue or a topic in the connection's namespace.
func (s *AzureServiceBusService) CreateEntity(connID int, input AzureServiceBusEntityInput) error {
	return s.service.CreateEntity(context.Background(), connID, input.spec())
}

// UpdateEntity changes an existing entity's settings. Sessions, duplicate
// detection and partitioning are fixed at creation and are not among them.
func (s *AzureServiceBusService) UpdateEntity(connID int, input AzureServiceBusEntityInput) error {
	return s.service.UpdateEntity(context.Background(), connID, input.spec())
}

// RemoveEntity deletes a queue or a topic, and everything it was holding.
// A topic takes every subscription on it, and their backlogs with them.
func (s *AzureServiceBusService) RemoveEntity(connID int, name string) error {
	return s.service.RemoveEntity(context.Background(), connID, name)
}

// AzureServiceBusSubscriptionInput is a subscription as its form collects it.
//
// Deliberately not ConsumerService.Create's shape. That one takes a cluster, a
// broker address, a consume mode and a retry count - RocketMQ's vocabulary, of
// which a Service Bus subscription has none. What it has is the same delivery
// contract a queue has, because on this family a subscription is where the
// messages actually are.
//
// What is not here is what reaches it: a new subscription comes with a
// $Default rule matching everything, and narrowing that is the routing page's
// job. A rule is an object with a name, so putting one in this form would hide
// a second write inside the first.
type AzureServiceBusSubscriptionInput struct {
	// Topic is required on a create and fixed afterwards: a subscription reads
	// exactly one topic, chosen when it is made.
	Topic string `json:"topic"`
	Name  string `json:"name"`

	LockDurationSec     int `json:"lockDurationSec"`
	MaxDeliveryCount    int `json:"maxDeliveryCount"`
	TTLSec              int `json:"ttlSec"`
	AutoDeleteOnIdleSec int `json:"autoDeleteOnIdleSec"`

	DeadLetterOnExpiry bool `json:"deadLetterOnExpiry"`
	// DeadLetterOnRuleError moves a message aside when a rule's expression
	// fails to evaluate. Without it such a message is discarded silently.
	DeadLetterOnRuleError bool `json:"deadLetterOnRuleError"`

	// RequiresSession is fixed at creation: the service refuses it in an
	// update, so the form only offers it on a create.
	RequiresSession bool `json:"requiresSession"`

	ForwardTo            string `json:"forwardTo"`
	ForwardDeadLettersTo string `json:"forwardDeadLettersTo"`
}

func (input AzureServiceBusSubscriptionInput) spec() servicebusdriver.SubscriptionSpec {
	return servicebusdriver.SubscriptionSpec{
		Topic:                 input.Topic,
		Name:                  input.Name,
		LockDurationSec:       input.LockDurationSec,
		MaxDeliveryCount:      input.MaxDeliveryCount,
		TTLSec:                input.TTLSec,
		AutoDeleteOnIdleSec:   input.AutoDeleteOnIdleSec,
		DeadLetterOnExpiry:    input.DeadLetterOnExpiry,
		DeadLetterOnRuleError: input.DeadLetterOnRuleError,
		RequiresSession:       input.RequiresSession,
		ForwardTo:             input.ForwardTo,
		ForwardDeadLettersTo:  input.ForwardDeadLettersTo,
	}
}

// CreateSubscription declares a subscription on a topic.
func (s *AzureServiceBusService) CreateSubscription(
	connID int, input AzureServiceBusSubscriptionInput,
) error {
	return s.service.CreateSubscription(context.Background(), connID, input.spec())
}

// UpdateSubscription changes what a subscription lets be changed. The topic
// and sessions are fixed at creation and are not among them.
func (s *AzureServiceBusService) UpdateSubscription(
	connID int, input AzureServiceBusSubscriptionInput,
) error {
	return s.service.UpdateSubscription(context.Background(), connID, input.spec())
}

// RemoveSubscription deletes a subscription and everything it had not
// delivered. Those messages were never the topic's to hand out again.
func (s *AzureServiceBusService) RemoveSubscription(connID int, topic, name string) error {
	return s.service.RemoveSubscription(context.Background(), connID, topic, name)
}
