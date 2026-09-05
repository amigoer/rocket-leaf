package bridge

import (
	"context"

	"github.com/amigoer/mq-studio/internal/model"
	activemqservice "github.com/amigoer/mq-studio/internal/service/activemq"
)

// ActiveMQService is the renderer's entry point for the operations only
// ActiveMQ has. Queues and topics, durable subscribers, browsing and sending
// all go through the canonical services; what is here is the rest.
type ActiveMQService struct {
	service *activemqservice.Service
}

// NewActiveMQService wires the bridge to the service.
func NewActiveMQService(service *activemqservice.Service) *ActiveMQService {
	return &ActiveMQService{service: service}
}

// PurgeQueue drops everything a destination is holding. There is no undo.
//
// On an Artemis topic this empties every subscription under the address rather
// than the address itself, which holds nothing - a call against the address
// would report success and change nothing.
func (s *ActiveMQService) PurgeQueue(connID int, name string) error {
	return s.service.PurgeQueue(context.Background(), connID, model.DestinationRef{Name: name})
}

// MoveInput names one destination to drain into another.
//
// Flatter than the canonical MoveRequest because ActiveMQ has no exchange and
// no routing key: a JMS move puts the message in the named destination, with
// no topology in between for it to take.
type ActiveMQMoveInput struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// MoveMessages drains one destination into another and reports how many the
// broker moved. The count is the broker's own, which is what separates a move
// that matched nothing from one that moved everything.
func (s *ActiveMQService) MoveMessages(connID int, input ActiveMQMoveInput) (int, error) {
	return s.service.MoveMessages(context.Background(), connID, model.MoveRequest{
		From:         input.From,
		ToRoutingKey: input.To,
	})
}

// ActiveMQDestinationInput is a destination declaration as the ActiveMQ form
// collects it.
//
// Deliberately not TopicService.Create's shape. That one takes a broker
// address, a read queue count, a write queue count and a permission string,
// which is RocketMQ's vocabulary; a JMS destination has none of those, and a
// form that filled them in with placeholders would be lying about what it
// sent.
type ActiveMQDestinationInput struct {
	Name string `json:"name"`
	// Topic rather than a kind string, because there are exactly two and a
	// boolean cannot arrive misspelled. It is not inferable from the name: a
	// queue and a topic may both be called ORDERS, and on Classic they are
	// different objects in different trees.
	Topic bool `json:"topic"`
}

// CreateDestination declares a queue or a topic.
func (s *ActiveMQService) CreateDestination(connID int, input ActiveMQDestinationInput) error {
	return s.service.CreateDestination(context.Background(), connID, input.Name, input.Topic)
}

// RemoveDestination deletes a destination and everything it holds.
func (s *ActiveMQService) RemoveDestination(connID int, name string) error {
	return s.service.RemoveDestination(context.Background(), connID, name)
}

// ActiveMQSubscriptionInput is a durable subscription as the ActiveMQ form
// collects it.
//
// Deliberately not ConsumerInput's shape: that one carries a broker address, a
// consume mode and a retry count, and carries no topic - which is the one
// thing a durable subscription cannot be made without.
type ActiveMQSubscriptionInput struct {
	// Topic is what the subscription reads.
	Topic string `json:"topic"`
	// Name is the canonical ref. On Artemis it is the queue's name; on Classic
	// it is the client id and the subscription name joined by a vertical bar,
	// because a JMS client id routinely contains a slash or a colon.
	Name string `json:"name"`
	// Selector is a JMS selector expression, empty for everything. Classic
	// rejects an empty string as an expression, so the driver sends null.
	Selector string `json:"selector"`
}

// CreateSubscription registers a durable subscription on a topic.
func (s *ActiveMQService) CreateSubscription(connID int, input ActiveMQSubscriptionInput) error {
	return s.service.CreateSubscription(context.Background(), connID,
		input.Topic, input.Name, input.Selector)
}

// RemoveSubscription unsubscribes, discarding whatever it was still owed.
func (s *ActiveMQService) RemoveSubscription(connID int, name string) error {
	return s.service.RemoveSubscription(context.Background(), connID, name)
}
