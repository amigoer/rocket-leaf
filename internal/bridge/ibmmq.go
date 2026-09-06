package bridge

import (
	"context"

	ibmmqdriver "github.com/amigoer/mq-studio/internal/driver/ibmmq"
	"github.com/amigoer/mq-studio/internal/model"
	ibmmqservice "github.com/amigoer/mq-studio/internal/service/ibmmq"
)

// IBMMQService is the renderer's entry point for the operations only IBM MQ
// has. Listing and describing queues and topics go through the canonical
// services; what is here is the rest.
type IBMMQService struct {
	service *ibmmqservice.Service
}

// NewIBMMQService wires the bridge to the service.
func NewIBMMQService(service *ibmmqservice.Service) *IBMMQService {
	return &IBMMQService{service: service}
}

// QueueManager is which queue manager this connection speaks to.
func (s *IBMMQService) QueueManager(connID int) (string, error) {
	return s.service.QueueManager(connID)
}

// IBMMQDestinationInput is a queue or a topic as the form collects it.
//
// Deliberately not TopicService.Create's shape. That one takes a broker
// address, two queue counts and a permission string - RocketMQ's vocabulary,
// of which neither an MQ queue nor an MQ topic has any.
type IBMMQDestinationInput struct {
	Name string `json:"name"`
	// Kind is "queue" or "topic", and it decides which interface the create
	// goes through: a queue is a REST resource and a topic is MQSC.
	Kind string `json:"kind"`

	// QueueType is local, alias, remote or model. Only a local queue stores
	// anything; the rest resolve somewhere else.
	QueueType string `json:"queueType"`
	// MaxDepth caps how many messages a local queue will hold. Zero leaves the
	// queue manager's own default, which is 5000 on a fresh installation.
	MaxDepth int `json:"maxDepth"`

	// TopicString is what publishers name, and it is required on a topic. It
	// is not the object's name: the object is where that string's settings are
	// attached, and two objects covering overlapping strings is ordinary.
	TopicString string `json:"topicString"`

	Description string `json:"description"`
}

func (input IBMMQDestinationInput) spec() model.DestinationSpec {
	attributes := map[string]string{
		ibmmqdriver.AttrKind:        input.Kind,
		ibmmqdriver.AttrQueueType:   input.QueueType,
		ibmmqdriver.AttrTopicString: input.TopicString,
		ibmmqdriver.AttrDescription: input.Description,
	}
	if input.MaxDepth > 0 {
		attributes[ibmmqdriver.AttrMaxDepth] = itoa(input.MaxDepth)
	}
	return model.DestinationSpec{
		Ref:        model.DestinationRef{Name: input.Name},
		Attributes: attributes,
	}
}

// CreateDestination declares a queue or a topic on the queue manager.
func (s *IBMMQService) CreateDestination(connID int, input IBMMQDestinationInput) error {
	return s.service.CreateDestination(context.Background(), connID, input.spec())
}

// RemoveDestination deletes a queue or a topic.
//
// purge decides what happens to a queue that is not empty: without it the
// queue manager refuses, with it the messages go too and there is no undo.
// A queue an application has open is refused either way.
func (s *IBMMQService) RemoveDestination(connID int, name string, purge bool) error {
	return s.service.RemoveDestination(context.Background(), connID, name, purge)
}

// Channels lists every channel definition and what is running on it.
//
// Definitions rather than connections, and the difference is the page: a
// channel is listed whether or not anything is using it, because it is what
// decides whether anything may - so a queue manager whose applications are all
// idle still has rows here, which is exactly when somebody is looking for why
// they cannot connect.
func (s *IBMMQService) Channels(connID int) ([]*model.Channel, error) {
	return s.service.Channels(context.Background(), connID)
}

// IBMMQPublishInput is a send as the IBM MQ console collects it.
//
// Deliberately not MessageService.Send's shape. That one takes a topic, tags,
// keys and a delay level; an MQ message has a descriptor instead - a
// correlation identifier, a persistence, an expiry and whatever properties the
// sender attached - and nothing anywhere in the queue manager holds a message
// back until later.
type IBMMQPublishInput struct {
	// Queue, not a topic: the messaging REST API has no topic resource at all,
	// so publishing needs an MQ client and this console cannot offer it.
	Queue string `json:"queue"`
	Body  string `json:"body"`
	// ContentType reaches the message descriptor's format. It has to be a
	// character type - the server refuses anything else outright.
	ContentType string `json:"contentType"`
	// CorrelationID is 48 hexadecimal characters or empty.
	CorrelationID string `json:"correlationId"`
	// Persistent decides whether the queue manager writes the message to its
	// log, and so whether it survives a restart.
	Persistent bool `json:"persistent"`
	// ExpirySeconds discards the message if nothing has read it by then. Zero
	// is MQ's own unlimited.
	ExpirySeconds int `json:"expirySeconds"`
	// Properties are attached under their own names, and are what a receiving
	// application reads back as message properties.
	Properties map[string]string `json:"properties"`
	// Count sends the same body more than once. Each copy is its own request.
	Count int `json:"count"`
}

// IBMMQPublishResult is what the send did.
type IBMMQPublishResult struct {
	Sent int `json:"sent"`
	// MessageID is the first message's, as the queue manager assigned it. It
	// is the handle the browse lists, so it is worth handing straight back.
	MessageID string `json:"messageId"`
}

// Publish sends to one queue and reports what the queue manager took.
func (s *IBMMQService) Publish(connID int, input IBMMQPublishInput) (*IBMMQPublishResult, error) {
	result, err := s.service.Publish(context.Background(), connID, ibmmqdriver.PublishRequest{
		Queue:         input.Queue,
		Body:          input.Body,
		ContentType:   input.ContentType,
		CorrelationID: input.CorrelationID,
		Persistent:    input.Persistent,
		ExpirySeconds: input.ExpirySeconds,
		Properties:    input.Properties,
		Count:         input.Count,
	})
	if err != nil {
		return nil, err
	}
	return &IBMMQPublishResult{Sent: result.Sent, MessageID: result.MessageID}, nil
}

// DeadLetterQueues lists the queues something else dead-letters into.
//
// Found by walking every queue's configuration backwards rather than by
// looking up a name: nothing marks a dead-letter queue on this family, and
// what makes one is the queue manager's DEADQ attribute or another queue's
// backout queue pointing at it.
func (s *IBMMQService) DeadLetterQueues(connID int) ([]*model.DeadLetterQueue, error) {
	return s.service.DeadLetters(context.Background(), connID)
}
