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
