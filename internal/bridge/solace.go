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
