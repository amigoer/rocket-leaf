package azureservicebus

import (
	"context"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * EntitySpec is a queue or a topic as the entity form collects it.
 *
 * Deliberately not TopicService.Create's shape. That one takes a broker
 * address, a read queue count, a write queue count and a permission string -
 * RocketMQ's vocabulary, of which a Service Bus entity has none. What it has
 * instead is a delivery contract: how long a receiver holds a message, how
 * many times it may be tried before it is dead-lettered, how long it lives at
 * all, and where it goes afterwards.
 *
 * One struct for both kinds, because the form is one form: a topic simply
 * ignores the delivery half, which belongs to its subscriptions. Kind decides
 * which half of the API is called and cannot be changed on an edit - a queue
 * and a topic are different objects, and turning one into the other means
 * deleting it and losing whatever it held.
 */
type EntitySpec struct {
	Name string
	// Kind is "queue" or "topic". Empty means a queue, which is the ordinary
	// case and what a form that forgot the field meant.
	Kind string

	// LockDurationSec is how long a receiver holds a message before Service
	// Bus offers it to somebody else. Zero leaves it alone, which on a create
	// takes the service's default of one minute. Queues only.
	LockDurationSec int
	// MaxDeliveryCount is how many times a message may be delivered before it
	// is moved to the dead-letter queue. Queues only; the default is ten.
	MaxDeliveryCount int
	// TTLSec is how long a message lives whether or not anything reads it.
	TTLSec int
	// AutoDeleteOnIdleSec deletes the entity itself after that long with no
	// traffic. Zero leaves it alone, which means never.
	AutoDeleteOnIdleSec int
	MaxSizeMB           int

	// DeadLetterOnExpiry moves an expired message to the dead-letter queue
	// instead of discarding it. Queues only.
	DeadLetterOnExpiry bool

	// The three below are fixed at creation: the service refuses them in an
	// update, so they are sent on a create and left out afterwards.
	RequiresSession            bool
	RequiresDuplicateDetection bool
	Partitioned                bool

	// ForwardTo hands every arriving message straight to another entity, which
	// is how a chain is built without a consumer in the middle.
	ForwardTo string
	// ForwardDeadLettersTo does the same for what this entity gives up on, and
	// is the only way to collect several entities' dead letters in one place.
	ForwardDeadLettersTo string
}

// spec turns the form's fields into the canonical destination spec, which is
// what keeps the attribute keys private to this package.
func (s EntitySpec) spec() model.DestinationSpec {
	kind := EntityQueue
	if strings.TrimSpace(s.Kind) == EntityTopic {
		kind = EntityTopic
	}
	attributes := map[string]string{
		AttrEntityType:              kind,
		AttrRequiresSession:         strconv.FormatBool(s.RequiresSession),
		AttrRequiresDuplicateDetect: strconv.FormatBool(s.RequiresDuplicateDetection),
		AttrPartitioned:             strconv.FormatBool(s.Partitioned),
		AttrDeadLetterOnExpiry:      strconv.FormatBool(s.DeadLetterOnExpiry),
		// Always carried, empty included: an empty value is how forwarding is
		// removed, and an absent attribute is how it is left alone.
		AttrForwardTo:            strings.TrimSpace(s.ForwardTo),
		AttrForwardDeadLettersTo: strings.TrimSpace(s.ForwardDeadLettersTo),
	}
	putPositive(attributes, AttrLockDurationSec, s.LockDurationSec)
	putPositive(attributes, AttrMaxDeliveryCount, s.MaxDeliveryCount)
	putPositive(attributes, AttrTTLSec, s.TTLSec)
	putPositive(attributes, AttrAutoDeleteOnIdleSec, s.AutoDeleteOnIdleSec)
	putPositive(attributes, AttrMaxSizeMB, s.MaxSizeMB)

	return model.DestinationSpec{
		Ref:        model.DestinationRef{Name: strings.TrimSpace(s.Name)},
		Attributes: attributes,
	}
}

// putPositive writes a value only when the form set one. Zero stays out, which
// is what makes an omitted setting mean "keep it" on an edit and "take the
// service's default" on a create.
func putPositive(attributes map[string]string, key string, value int) {
	if value > 0 {
		attributes[key] = strconv.Itoa(value)
	}
}

// CreateEntity declares a queue or a topic from a form submission.
func (c *Conn) CreateEntity(ctx context.Context, spec EntitySpec) error {
	return c.CreateDestination(ctx, spec.spec())
}

// UpdateEntity changes what an existing entity lets be changed.
func (c *Conn) UpdateEntity(ctx context.Context, spec EntitySpec) error {
	return c.UpdateDestination(ctx, spec.spec())
}
