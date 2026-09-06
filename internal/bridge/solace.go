package bridge

import (
	"context"

	solacedriver "github.com/amigoer/mq-studio/internal/driver/solace"
	"github.com/amigoer/mq-studio/internal/model"
	solaceservice "github.com/amigoer/mq-studio/internal/service/solace"
)

// SolaceService is the renderer's entry point for the operations only Solace
// has. Listing and describing queues go through the canonical services; what
// is here is the rest.
type SolaceService struct {
	service *solaceservice.Service
}

// NewSolaceService wires the bridge to the service.
func NewSolaceService(service *solaceservice.Service) *SolaceService {
	return &SolaceService{service: service}
}

// MsgVPN is which Message VPN this connection reads.
func (s *SolaceService) MsgVPN(connID int) (string, error) {
	return s.service.MsgVPN(connID)
}

// SolaceQueueInput is a queue as the form collects it.
//
// Deliberately not TopicService.Create's shape. That one takes a broker
// address, two queue counts and a permission string - RocketMQ's vocabulary,
// of which a Solace queue has none.
type SolaceQueueInput struct {
	Name string `json:"name"`

	// AccessType is "exclusive" or "non-exclusive", and it is the setting most
	// likely to be wrong: exclusive hands every message to one consumer and
	// keeps the rest waiting as standbys, which looks like a broken fan-out
	// rather than like a configuration choice.
	AccessType string `json:"accessType"`
	// Permission is what a client bound to this queue may do to it - read,
	// consume, modify its topics, or delete it.
	Permission string `json:"permission"`

	// Owner is the client username the queue belongs to. Empty leaves it
	// unowned, which is what a queue created by an administrator usually is.
	Owner string `json:"owner"`

	// DeadMsgQueue is where undelivered messages go. Naming one also turns off
	// respectDmqEligible, because otherwise only a message its publisher
	// marked eligible is ever moved - and nothing this app sends is marked.
	DeadMsgQueue string `json:"deadMsgQueue"`
	// MaxRedeliveryCount is how many attempts before that happens. Zero is the
	// broker's own unlimited.
	MaxRedeliveryCount int `json:"maxRedeliveryCount"`

	// MaxSpoolUsageMb caps what the queue may hold, in megabytes. Zero leaves
	// the broker's own default.
	MaxSpoolUsageMb int `json:"maxSpoolUsageMb"`
}

func (input SolaceQueueInput) spec() model.DestinationSpec {
	attributes := map[string]string{
		solacedriver.AttrAccessType: input.AccessType,
		solacedriver.AttrPermission: input.Permission,
		solacedriver.AttrOwner:      input.Owner,
	}
	if input.DeadMsgQueue != "" {
		attributes[solacedriver.AttrDeadMsgQueue] = input.DeadMsgQueue
	}
	if input.MaxRedeliveryCount > 0 {
		attributes[solacedriver.AttrMaxRedelivery] = itoa(input.MaxRedeliveryCount)
	}
	if input.MaxSpoolUsageMb > 0 {
		attributes[solacedriver.AttrMaxSpool] = itoa(input.MaxSpoolUsageMb)
	}
	return model.DestinationSpec{
		Ref:        model.DestinationRef{Name: input.Name},
		Attributes: attributes,
	}
}

// CreateQueue declares a queue in this connection's Message VPN.
func (s *SolaceService) CreateQueue(connID int, input SolaceQueueInput) error {
	return s.service.CreateDestination(context.Background(), connID, input.spec())
}

// RemoveQueue deletes a queue and whatever it was holding.
//
// No purge flag: SEMP has no precondition to ask for, and it deletes a full
// queue as readily as an empty one. The confirmation on the board says so
// rather than this offering a guard the broker does not have.
func (s *SolaceService) RemoveQueue(connID int, name string) error {
	return s.service.RemoveDestination(context.Background(), connID, name)
}

// SolacePublishInput is a send as the Solace console collects it.
//
// Deliberately not MessageService.Send's shape. That one takes a topic, tags,
// keys and a delay level; a Solace message has a delivery mode, a time to live
// and a dead-message flag instead, and it can go to a queue by name or to a
// topic to be matched - which are two different things and not one field with
// two spellings.
type SolacePublishInput struct {
	// Target is "queue" or "topic". A queue send names one endpoint; a topic
	// send is matched against every subscription in the Message VPN and lands
	// on nothing when none match.
	Target      string `json:"target"`
	Destination string `json:"destination"`
	Body        string `json:"body"`
	ContentType string `json:"contentType"`

	// DeliveryMode is "persistent", "non-persistent" or "direct". Empty is
	// persistent.
	DeliveryMode string `json:"deliveryMode"`
	// TimeToLiveMs discards the message if nothing takes it by then. Zero is
	// the broker's own unlimited.
	TimeToLiveMs int `json:"timeToLiveMs"`
	// DMQEligible decides whether a message given up on is moved to the
	// queue's dead message queue or discarded. The broker's default is off,
	// which is why a queue configured to dead-letter can still discard.
	DMQEligible   bool              `json:"dmqEligible"`
	CorrelationID string            `json:"correlationId"`
	ReplyTo       string            `json:"replyTo"`
	Properties    map[string]string `json:"properties"`
	// Count sends the same body more than once. Each copy is its own request.
	Count int `json:"count"`
}

// SolacePublishResult is what the send did.
//
// There is no message id, and that is the interface rather than an omission:
// the broker answers a successful send with an empty body and no identifier.
// The id a browse lists is the queue's own sequence number, assigned when the
// message is spooled and never told to the publisher.
type SolacePublishResult struct {
	Sent int `json:"sent"`
}

// Publish sends to one queue or one topic and reports what the broker took.
func (s *SolaceService) Publish(connID int, input SolacePublishInput) (*SolacePublishResult, error) {
	result, err := s.service.Publish(context.Background(), connID, solacedriver.PublishRequest{
		Target:        input.Target,
		Destination:   input.Destination,
		Body:          input.Body,
		ContentType:   input.ContentType,
		DeliveryMode:  input.DeliveryMode,
		TimeToLiveMs:  input.TimeToLiveMs,
		DMQEligible:   input.DMQEligible,
		CorrelationID: input.CorrelationID,
		ReplyTo:       input.ReplyTo,
		Properties:    input.Properties,
		Count:         input.Count,
	})
	if err != nil {
		return nil, err
	}
	return &SolacePublishResult{Sent: result.Sent}, nil
}

// DeadMsgQueues lists the queues something else dead-letters into.
//
// Found by walking every endpoint's configuration backwards rather than by
// looking up a name: nothing marks a dead message queue on this family, and
// what makes one is another queue's or topic endpoint's pointer. A target with
// no depth is one the Message VPN does not hold - which is the ordinary state
// of the pointer every endpoint ships with, and what makes a message given up
// on disappear rather than move.
func (s *SolaceService) DeadMsgQueues(connID int) ([]*model.DeadLetterQueue, error) {
	return s.service.DeadLetters(context.Background(), connID)
}

// Subscribe adds a topic subscription to a queue.
//
// Two arguments rather than a binding, because there is no exchange between a
// topic and a queue here: the source, the routing key and the handle are all
// the one topic string.
//
// Nothing already spooled moves. A subscription added now attracts what is
// published from now on.
func (s *SolaceService) Subscribe(connID int, queue, topic string) error {
	return s.service.Subscribe(context.Background(), connID, queue, topic)
}

// Unsubscribe drops a topic subscription from a queue. What it already brought
// stays where it is.
func (s *SolaceService) Unsubscribe(connID int, queue, topic string) error {
	return s.service.Unsubscribe(context.Background(), connID, queue, topic)
}

// CreateTopicEndpoint declares an endpoint whose name is its subscription.
//
// No other field: a topic endpoint has no exchange type and nothing to decide
// beyond the topic it is called after.
func (s *SolaceService) CreateTopicEndpoint(connID int, name string) error {
	return s.service.CreateTopicEndpoint(context.Background(), connID, name)
}

// RemoveTopicEndpoint deletes one, and whatever it was holding.
func (s *SolaceService) RemoveTopicEndpoint(connID int, name string) error {
	return s.service.RemoveTopicEndpoint(context.Background(), connID, name)
}

// Clients lists what is holding a session open on this Message VPN.
//
// The broker's own machinery is in the list rather than filtered out of it: a
// client named with a leading "#" is the broker talking to itself, and it is
// marked so a reader counting applications can leave it out. Hiding them would
// hide connections that hold real resources.
func (s *SolaceService) Clients(connID int) ([]*model.ClientConnection, error) {
	return s.service.Clients(context.Background(), connID)
}
