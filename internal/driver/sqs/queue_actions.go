package sqs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * PurgeQueue discards everything the queue holds.
 *
 * Three things about it are the service's rather than this driver's, and all
 * three are worth knowing before the button is pressed. It is asynchronous, so
 * the call returning is not the queue being empty. Anything sent in the minute
 * after it may be deleted too, and anything sent before it may still be
 * delivered for up to a minute after. And a second purge inside 60 seconds is
 * refused outright, which is reported here as what it is rather than as a
 * failure to empty the queue.
 */
func (c *Conn) PurgeQueue(ctx context.Context, ref model.DestinationRef) error {
	if err := c.live(); err != nil {
		return err
	}
	url, err := c.queueURL(ctx, ref.Name)
	if err != nil {
		return err
	}
	_, err = c.client.PurgeQueue(ctx, &awssqs.PurgeQueueInput{QueueUrl: aws.String(url)})
	var inProgress *types.PurgeQueueInProgress
	if errors.As(err, &inProgress) {
		return fmt.Errorf(
			"%s was purged less than a minute ago; SQS allows one purge per queue per minute: %w",
			ref.Name, err)
	}
	return err
}

// MoveMessages is not offered. See errNoMove.
func (c *Conn) MoveMessages(context.Context, model.MoveRequest) (int, error) {
	return 0, errNoMove
}

// DropMessages is not offered. See errNoDrop.
func (c *Conn) DropMessages(context.Context, model.DestinationRef, int) (int, error) {
	return 0, errNoDrop
}

// RebalanceQueues is not offered. See errNoRebalance.
func (c *Conn) RebalanceQueues(context.Context) error {
	return errNoRebalance
}

// The three QueueActions methods this family has no counterpart for.
//
// They exist because QueueActions is one interface; none of their capabilities
// is declared, so nothing in the UI reaches them. The reasons are in
// conformance_test.go, which is where they are asserted.
var (
	errNoMove = errors.New(
		"sqs cannot drain one queue into another on demand; a redrive task moves a " +
			"dead-letter queue back to the queues its messages came from, which is a " +
			"repair with a fixed destination rather than a move")
	errNoDrop = errors.New(
		"sqs has no way to discard a bounded batch from a queue's head; a purge " +
			"empties the whole queue and is the only thing that removes messages nobody read")
	errNoRebalance = errors.New(
		"aws places an sqs queue; there is no node here to spread anything across")
)

/*
 * QueueSpec is a queue as the queue form collects it.
 *
 * Deliberately not TopicService.Create's shape. That one takes a broker
 * address, a read queue count, a write queue count and a permission string -
 * RocketMQ's vocabulary, of which an SQS queue has none. What it has instead is
 * a set of durations the service enforces, and a redrive policy naming another
 * queue by name rather than by the ARN the API wants.
 */
type QueueSpec struct {
	Name string
	// FIFO is fixed at creation and spelled in the name. It is carried
	// separately so a form can refuse the mismatch before the service does,
	// with a message naming the field the user actually filled in.
	FIFO bool

	// Zero means "leave it alone", which is what lets an edit change one
	// setting without reading and rewriting the others. On a create the
	// service applies its own defaults to whatever is left out.
	VisibilityTimeoutSec int
	DelaySec             int
	RetentionSec         int
	MaxMessageBytes      int
	ReceiveWaitSec       int

	// DeadLetterQueue is another queue's name. The API wants its ARN, which
	// the driver resolves - the name is what a person picks from a list.
	DeadLetterQueue string
	MaxReceiveCount int

	// FIFO-only, and ignored on a standard queue rather than refused: SQS
	// answers "Unknown Attribute" for each of them, naming a field the form
	// never drew.
	ContentBasedDeduplication bool
	DeduplicationScope        string
	FifoThroughputLimit       string
}

// spec turns the form's fields into the canonical destination spec, which is
// what keeps the attribute keys private to this package.
func (q QueueSpec) spec() model.DestinationSpec {
	attributes := map[string]string{
		AttrFIFO: boolAttr(q.FIFO),
	}
	setPositive(attributes, AttrVisibilityTimeo, q.VisibilityTimeoutSec)
	// Zero is a real delay - it is the default - so it cannot mean "unset"
	// here the way the others do. What distinguishes them is that a queue
	// cannot have a zero visibility timeout or retention at all.
	if q.DelaySec > 0 {
		setPositive(attributes, AttrDelaySeconds, q.DelaySec)
	}
	setPositive(attributes, AttrRetentionSec, q.RetentionSec)
	setPositive(attributes, AttrMaxMessageBytes, q.MaxMessageBytes)
	if q.ReceiveWaitSec > 0 {
		setPositive(attributes, AttrReceiveWaitSec, q.ReceiveWaitSec)
	}

	if strings.TrimSpace(q.DeadLetterQueue) != "" {
		attributes[AttrDeadLetterQueue] = strings.TrimSpace(q.DeadLetterQueue)
		setPositive(attributes, AttrMaxReceiveCount, q.MaxReceiveCount)
	}
	if q.FIFO {
		attributes[AttrContentDedup] = boolAttr(q.ContentBasedDeduplication)
		if q.DeduplicationScope != "" {
			attributes[AttrDedupScope] = q.DeduplicationScope
		}
		if q.FifoThroughputLimit != "" {
			attributes[AttrThroughputLimit] = q.FifoThroughputLimit
		}
	}

	return model.DestinationSpec{
		Ref:        model.DestinationRef{Name: strings.TrimSpace(q.Name)},
		Attributes: attributes,
	}
}

func setPositive(attributes map[string]string, key string, value int) {
	if value > 0 {
		attributes[key] = itoa(value)
	}
}

func boolAttr(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func itoa(value int) string { return strconv.Itoa(value) }

// CreateQueue declares a queue from a form submission.
func (c *Conn) CreateQueue(ctx context.Context, spec QueueSpec) error {
	return c.CreateDestination(ctx, spec.spec())
}

// UpdateQueue changes an existing queue's settings.
func (c *Conn) UpdateQueue(ctx context.Context, spec QueueSpec) error {
	return c.UpdateDestination(ctx, spec.spec())
}
