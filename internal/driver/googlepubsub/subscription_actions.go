package googlepubsub

import (
	"context"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * SubscriptionSpec is a subscription as the subscription form collects it.
 *
 * Deliberately not ConsumerGroupConfig's shape. That one takes a cluster, a
 * broker address, a consume mode, a retry count and a list of topics -
 * RocketMQ's vocabulary, of which a Pub/Sub subscription has exactly one
 * topic and nothing else. What it has instead is the whole of the delivery
 * configuration, because on this family that belongs to the subscription
 * rather than to the topic it reads.
 *
 * Four fields are fixed at creation and are why this is not simply an update
 * with a name: the topic, the filter, message ordering and the name itself.
 */
type SubscriptionSpec struct {
	Name string
	// Topic is required on a create and ignored on an update: a subscription
	// reads exactly one topic, chosen when it is made.
	Topic string

	// AckDeadlineSec is how long a delivered message is held before Pub/Sub
	// offers it again. Zero leaves it alone, which on a create takes the
	// service's default of ten seconds.
	AckDeadlineSec int
	// RetentionSec is how long an unacknowledged message is kept. Zero leaves
	// it alone; the service's default is seven days.
	RetentionSec int
	// RetainAcked keeps acknowledged messages inside the retention window, so
	// a seek can go back past them. Without it a seek can only skip forward.
	RetainAcked bool
	// ExactlyOnce turns on the stronger delivery guarantee, which also makes
	// the acknowledgement itself something that can fail and be retried.
	ExactlyOnce bool

	// Filter is a Pub/Sub filter expression over message attributes. Set at
	// creation and never afterwards: the service refuses the update, and so
	// does the emulator with a message naming the emulator.
	Filter string
	// Ordering delivers messages with the same ordering key in order. Also
	// fixed at creation.
	Ordering bool

	// PushEndpoint makes this a push subscription, which Pub/Sub POSTs to
	// rather than holding for a reader. Empty is an ordinary pull one.
	PushEndpoint string

	// DeadLetterTopic is another topic's name; the driver resolves its path.
	// Empty removes the policy on an update, which is the only way to.
	DeadLetterTopic string
	MaxAttempts     int

	// RetryMinBackoffSec and RetryMaxBackoffSec are sent together or not at
	// all: the service refuses a policy carrying one of them.
	RetryMinBackoffSec int
	RetryMaxBackoffSec int

	// Labels are set at creation only. The emulator refuses them in an update
	// mask, which the real API accepts - and a form offering an edit that
	// fails against the reference environment is worse than one that does not.
	Labels map[string]string
}

// createSpec is the whole form, for a create.
func (s SubscriptionSpec) createSpec() model.SubscriptionSpec {
	attributes := map[string]string{
		SubAttrTopic:       strings.TrimSpace(s.Topic),
		SubAttrRetainAcked: strconv.FormatBool(s.RetainAcked),
		SubAttrExactlyOnce: strconv.FormatBool(s.ExactlyOnce),
		SubAttrOrdering:    strconv.FormatBool(s.Ordering),
	}
	if filter := strings.TrimSpace(s.Filter); filter != "" {
		attributes[SubAttrFilter] = filter
	}
	if endpoint := strings.TrimSpace(s.PushEndpoint); endpoint != "" {
		attributes[SubAttrPushEndpoint] = endpoint
	}
	for key, value := range s.Labels {
		if strings.TrimSpace(key) != "" {
			attributes[SubAttrLabelPrefix+key] = value
		}
	}
	s.addSettings(attributes)
	// A create names the dead-letter topic only when there is one: an empty
	// value means "remove it", which is meaningless before it exists.
	if strings.TrimSpace(s.DeadLetterTopic) == "" {
		delete(attributes, SubAttrDeadLetterTopic)
	}
	return model.SubscriptionSpec{
		Ref:        model.SubscriptionRef{Name: strings.TrimSpace(s.Name)},
		Attributes: attributes,
	}
}

// updateSpec is what an update may change, which is less.
//
// The dead-letter topic is always carried, empty included, because an empty
// one is how the policy is removed - the field mask names the field, so
// sending it with nothing inside clears it.
func (s SubscriptionSpec) updateSpec() model.SubscriptionSpec {
	attributes := map[string]string{
		SubAttrRetainAcked:     strconv.FormatBool(s.RetainAcked),
		SubAttrExactlyOnce:     strconv.FormatBool(s.ExactlyOnce),
		SubAttrDeadLetterTopic: strings.TrimSpace(s.DeadLetterTopic),
	}
	s.addSettings(attributes)
	return model.SubscriptionSpec{
		Ref:        model.SubscriptionRef{Name: strings.TrimSpace(s.Name)},
		Attributes: attributes,
	}
}

// addSettings writes the numeric fields a create and an update share. Zero
// stays out, which is what makes an omitted setting mean "keep it".
func (s SubscriptionSpec) addSettings(attributes map[string]string) {
	if s.AckDeadlineSec > 0 {
		attributes[SubAttrAckDeadlineSec] = strconv.Itoa(s.AckDeadlineSec)
	}
	if s.RetentionSec > 0 {
		attributes[SubAttrRetentionSec] = strconv.Itoa(s.RetentionSec)
	}
	if strings.TrimSpace(s.DeadLetterTopic) != "" {
		attributes[SubAttrDeadLetterTopic] = strings.TrimSpace(s.DeadLetterTopic)
		if s.MaxAttempts > 0 {
			attributes[SubAttrMaxAttempts] = strconv.Itoa(s.MaxAttempts)
		}
	}
	if s.RetryMinBackoffSec > 0 || s.RetryMaxBackoffSec > 0 {
		attributes[SubAttrRetryMinSec] = strconv.Itoa(s.RetryMinBackoffSec)
		attributes[SubAttrRetryMaxSec] = strconv.Itoa(s.RetryMaxBackoffSec)
	}
}

// CreateSubscriptionFrom declares a subscription from a form submission.
func (c *Conn) CreateSubscriptionFrom(ctx context.Context, spec SubscriptionSpec) error {
	return c.CreateSubscription(ctx, spec.createSpec())
}

// UpdateSubscriptionFrom changes what a subscription lets be changed.
func (c *Conn) UpdateSubscriptionFrom(ctx context.Context, spec SubscriptionSpec) error {
	return c.UpdateSubscription(ctx, spec.updateSpec())
}
