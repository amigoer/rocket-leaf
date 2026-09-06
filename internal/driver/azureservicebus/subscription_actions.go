package azureservicebus

import (
	"context"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * SubscriptionSpec is a subscription as its form collects it.
 *
 * Deliberately not ConsumerGroupConfig's shape. That one takes a cluster, a
 * broker address, a consume mode, a retry count and a list of topics -
 * RocketMQ's vocabulary, of which a Service Bus subscription has exactly one
 * topic and nothing else. What it has instead is the same delivery contract a
 * queue has, because on this family a subscription is where the messages are.
 *
 * What is not on this form is what reaches the subscription. A new one comes
 * with a $Default rule matching everything, and narrowing that is the routing
 * page's job - a rule is an object with a name, created and deleted on its
 * own, so putting one in this form would hide a second write inside the first.
 */
type SubscriptionSpec struct {
	// Topic is required on a create and fixed afterwards: a subscription
	// reads exactly one topic, chosen when it is made.
	Topic string
	Name  string

	LockDurationSec     int
	MaxDeliveryCount    int
	TTLSec              int
	AutoDeleteOnIdleSec int

	DeadLetterOnExpiry bool
	// DeadLetterOnRuleError moves a message aside when a rule's own expression
	// fails to evaluate, rather than discarding it. Without it a filter that
	// throws loses the message silently.
	DeadLetterOnRuleError bool

	// RequiresSession is fixed at creation: the service refuses it in an
	// update, so the form only offers it on a create.
	RequiresSession bool

	ForwardTo            string
	ForwardDeadLettersTo string
}

func (s SubscriptionSpec) spec() model.SubscriptionSpec {
	attributes := map[string]string{
		SubAttrRequiresSession:      strconv.FormatBool(s.RequiresSession),
		SubAttrDeadLetterOnExpiry:   strconv.FormatBool(s.DeadLetterOnExpiry),
		SubAttrDeadLetterOnRuleFail: strconv.FormatBool(s.DeadLetterOnRuleError),
		// Always carried, empty included: an empty value removes the
		// forwarding, and an absent attribute leaves it alone.
		SubAttrForwardTo:            strings.TrimSpace(s.ForwardTo),
		SubAttrForwardDeadLettersTo: strings.TrimSpace(s.ForwardDeadLettersTo),
	}
	putPositive(attributes, SubAttrLockDurationSec, s.LockDurationSec)
	putPositive(attributes, SubAttrMaxDeliveryCount, s.MaxDeliveryCount)
	putPositive(attributes, SubAttrTTLSec, s.TTLSec)
	putPositive(attributes, SubAttrAutoDeleteOnIdleSec, s.AutoDeleteOnIdleSec)

	return model.SubscriptionSpec{
		Ref: model.SubscriptionRef{
			Namespace: strings.TrimSpace(s.Topic),
			Name:      strings.TrimSpace(s.Name),
		},
		Attributes: attributes,
	}
}

// CreateSubscriptionFrom declares a subscription from a form submission.
func (c *Conn) CreateSubscriptionFrom(ctx context.Context, spec SubscriptionSpec) error {
	return c.CreateSubscription(ctx, spec.spec())
}

// UpdateSubscriptionFrom changes what a subscription lets be changed.
func (c *Conn) UpdateSubscriptionFrom(ctx context.Context, spec SubscriptionSpec) error {
	return c.UpdateSubscription(ctx, spec.spec())
}
