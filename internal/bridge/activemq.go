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

// DeadLetterQueues finds the destinations dead letters land in, and the
// destinations that feed them.
//
// The sources are empty on Classic and filled on Artemis, and that is the
// broker rather than the driver: Artemis records a dead-letter address on
// every queue, so the topology can be walked backwards; Classic decides by a
// broker-wide policy and keeps no record of where a dead letter came from.
func (s *ActiveMQService) DeadLetterQueues(connID int) ([]*model.DeadLetterQueue, error) {
	return s.service.DeadLetterQueues(context.Background(), connID)
}

// RetryDeadLetters sends a dead-lettered destination's contents back to the
// destinations each message originally failed on, and reports how many the
// broker moved.
//
// The whole destination, because that is the only form either product offers:
// retryMessages() takes no arguments on Classic or on Artemis.
func (s *ActiveMQService) RetryDeadLetters(connID int, name string) (int, error) {
	return s.service.RetryDeadLetters(context.Background(), connID, name)
}

// ActiveMQPublishInput is a send as the ActiveMQ console collects it.
//
// Flatter than the canonical PublishRequest, which is AMQP's: there is no
// exchange, no routing key and no mandatory flag here, because a JMS send
// names its destination and nothing routes in between. What is left is the
// body, the JMS headers a producer can set, and a count.
type ActiveMQPublishInput struct {
	Destination string `json:"destination"`
	Body        string `json:"body"`
	// Persistent is honoured on Artemis, whose sendMessage takes it. Classic's
	// sendTextMessage has no delivery-mode parameter and the destination's own
	// policy decides, so the switch does nothing there - which the console
	// says rather than pretending otherwise.
	Persistent    bool              `json:"persistent"`
	Priority      int               `json:"priority"`
	CorrelationID string            `json:"correlationId"`
	ReplyTo       string            `json:"replyTo"`
	JMSType       string            `json:"jmsType"`
	Headers       map[string]string `json:"headers"`
	Count         int               `json:"count"`
}

// Publish sends one or more messages and reports what the broker took.
func (s *ActiveMQService) Publish(connID int, input ActiveMQPublishInput) (*model.PublishResult, error) {
	return s.service.Publish(context.Background(), connID, model.PublishRequest{
		RoutingKey:    input.Destination,
		Body:          input.Body,
		Persistent:    input.Persistent,
		Priority:      input.Priority,
		CorrelationID: input.CorrelationID,
		ReplyTo:       input.ReplyTo,
		Type:          input.JMSType,
		Headers:       input.Headers,
		Count:         input.Count,
	})
}

// Connections lists what is holding a socket open on the broker.
//
// The protocol column is read differently on the two products and both are the
// broker's own answer: Artemis reports the connection's Java class, which the
// driver maps onto the protocol's name, and Classic reports which connector
// accepted the socket - which is the same fact, since a connector serves one
// protocol.
func (s *ActiveMQService) Connections(connID int) ([]*model.ClientConnection, error) {
	return s.service.Connections(context.Background(), connID)
}

// CloseConnection disconnects one client.
//
// By the broker's own connection id rather than by address, because an address
// is not unique - one host can hold twenty connections, and closing by address
// would take all of them.
func (s *ActiveMQService) CloseConnection(connID int, name string) error {
	return s.service.CloseConnection(context.Background(), connID, name)
}
