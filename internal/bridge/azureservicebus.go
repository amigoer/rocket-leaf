package bridge

import (
	"context"
	"time"

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

// AzureServiceBusSendInput is a send as the Service Bus console collects it.
//
// Deliberately not MessageService.Send's shape. That one takes a topic, tags,
// keys and a delay level - RocketMQ's vocabulary - and a Service Bus message
// carries rather more: a subject, a correlation id, a session or partition
// key, a content type, and a table of named properties.
//
// Those last two are not decoration here. A subscription's rules select on
// them: a SQL filter reads the properties by name and a correlation filter
// matches the subject and correlation id by equality, so a console that could
// not set them would make the routing page untestable from the app.
type AzureServiceBusSendInput struct {
	// Entity is a queue or a topic. A subscription cannot be sent to: it
	// receives what its topic copies into it.
	Entity string `json:"entity"`
	Body   string `json:"body"`
	// Count sends the same message more than once. One when left at zero.
	Count int `json:"count"`

	Subject       string `json:"subject"`
	CorrelationID string `json:"correlationId"`
	ContentType   string `json:"contentType"`
	SessionID     string `json:"sessionId"`
	PartitionKey  string `json:"partitionKey"`

	Properties map[string]string `json:"properties"`

	// DelaySec schedules the message for later instead of sending it now. It
	// becomes a real scheduled message: it sits in the entity until its time
	// comes, which the messages page shows and no consumer is offered.
	DelaySec int `json:"delaySec"`
	// TTLSec overrides the entity's own time to live for these messages only.
	TTLSec int `json:"ttlSec"`
}

// AzureServiceBusSendResult is what the send did.
//
// No message id, and its absence is the family: Service Bus's MessageId is the
// sender's own field, nothing assigns one and nothing indexes it. What
// addresses a message is its sequence number, and the service reports one only
// for a scheduled send - an immediate message is given its sequence on arrival
// and the sender is never told.
type AzureServiceBusSendResult struct {
	Sent int `json:"sent"`
	// SequenceNumbers are the scheduled messages' handles, which do address
	// something: cancelling a scheduled message takes one. Empty on an
	// immediate send.
	SequenceNumbers []int64 `json:"sequenceNumbers"`
}

// Send publishes to one entity and reports how many the service accepted.
//
// Accepted is not delivered. A queue holds what is sent to it; a topic holds
// nothing, copying the message into every subscription whose rules let it
// through and discarding it if none do - and reporting success either way.
func (s *AzureServiceBusService) Send(
	connID int, input AzureServiceBusSendInput,
) (*AzureServiceBusSendResult, error) {
	result, err := s.service.Send(context.Background(), connID, servicebusdriver.SendRequest{
		Entity:        input.Entity,
		Body:          input.Body,
		Count:         input.Count,
		Subject:       input.Subject,
		CorrelationID: input.CorrelationID,
		ContentType:   input.ContentType,
		SessionID:     input.SessionID,
		PartitionKey:  input.PartitionKey,
		Properties:    input.Properties,
		Delay:         time.Duration(input.DelaySec) * time.Second,
		TimeToLive:    time.Duration(input.TTLSec) * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return &AzureServiceBusSendResult{
		Sent:            result.Sent,
		SequenceNumbers: result.SequenceNumbers,
	}, nil
}

// CancelScheduled takes back messages that have not been enqueued yet.
func (s *AzureServiceBusService) CancelScheduled(
	connID int, entity string, sequences []int64,
) error {
	return s.service.CancelScheduled(context.Background(), connID, entity, sequences)
}

// AzureServiceBusRuleInput is a rule as the routing form collects it.
//
// A rule is what decides which of a topic's messages reach one subscription,
// and it is an object rather than a field: it has a name, several may sit on
// one subscription, and each is a filter of one of three kinds plus an
// optional action that rewrites the message on the way in.
type AzureServiceBusRuleInput struct {
	Topic        string `json:"topic"`
	Subscription string `json:"subscription"`
	// Name is what deletes it: one subscription may have several rules, and
	// nothing else tells them apart.
	Name string `json:"name"`

	// Kind is "sql", "correlation", "true" or "false". Empty means true,
	// which is what the service's own $Default rule is.
	Kind string `json:"kind"`

	// Expression is the SQL filter's text, on a sql rule.
	Expression string `json:"expression"`
	// Correlation is the message fields a correlation rule compares by
	// equality. A field left out matches anything.
	Correlation map[string]string `json:"correlation"`

	// Action is a SQL statement run on a matching message before it is copied
	// in - the half of a rule that changes the message rather than selecting
	// it. Optional on every kind.
	Action string `json:"action"`
}

// CreateRule declares a rule on a subscription.
func (s *AzureServiceBusService) CreateRule(connID int, input AzureServiceBusRuleInput) error {
	return s.service.CreateRule(context.Background(), connID, servicebusdriver.RuleSpec{
		Topic:        input.Topic,
		Subscription: input.Subscription,
		Name:         input.Name,
		Kind:         input.Kind,
		Expression:   input.Expression,
		Correlation:  input.Correlation,
		Action:       input.Action,
	})
}

// RemoveRule deletes one rule by name.
//
// Deleting the last one leaves a subscription nothing can reach: it stays
// Active, its backlog stays empty because nothing arrives, and only the
// subscriptions board's status says so.
func (s *AzureServiceBusService) RemoveRule(connID int, topic, subscription, name string) error {
	return s.service.RemoveRule(context.Background(), connID, topic, subscription, name)
}
