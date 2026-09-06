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
