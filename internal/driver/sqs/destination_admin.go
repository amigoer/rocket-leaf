package sqs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/amigoer/mq-studio/internal/model"
)

// Creating, reconfiguring and deleting queues.
//
// The settable attributes are named rather than passed through, because SQS
// refuses an attribute it does not know and answers with the name and nothing
// else - so a typo in a spec would surface as "Unknown Attribute" with no
// indication of where it came from. The spec's keys are this driver's own,
// which is what makes the mapping worth writing down.

// The attribute names SQS itself uses, keyed by the spec keys the pages send.
var settableAttributes = map[string]string{
	AttrVisibilityTimeo: "VisibilityTimeout",
	AttrDelaySeconds:    "DelaySeconds",
	AttrRetentionSec:    "MessageRetentionPeriod",
	AttrMaxMessageBytes: "MaximumMessageSize",
	AttrReceiveWaitSec:  "ReceiveMessageWaitTimeSeconds",
	AttrContentDedup:    "ContentBasedDeduplication",
	AttrDedupScope:      "DeduplicationScope",
	AttrThroughputLimit: "FifoThroughputLimit",
}

/*
 * CreateDestination declares a queue.
 *
 * FIFO is not a setting: SQS decides from the name, requires the .fifo suffix
 * on a FIFO queue and refuses it on a standard one. So the spec's fifo flag is
 * checked against the name rather than sent, and a disagreement is refused
 * here - the service's own message names an attribute the user never typed.
 */
func (c *Conn) CreateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	name := strings.TrimSpace(spec.Ref.Name)
	if name == "" {
		return errors.New("a queue needs a name")
	}
	if wantFIFO := spec.Attributes[AttrFIFO] == "true"; wantFIFO != isFIFO(name) {
		if wantFIFO {
			return fmt.Errorf("a FIFO queue's name has to end in .fifo; %q does not", name)
		}
		return fmt.Errorf("%q ends in .fifo, which SQS accepts only on a FIFO queue", name)
	}

	attributes, err := c.attributesOf(ctx, spec)
	if err != nil {
		return err
	}
	if isFIFO(name) {
		attributes["FifoQueue"] = "true"
	}

	out, err := c.client.CreateQueue(ctx, &awssqs.CreateQueueInput{
		QueueName:  aws.String(name),
		Attributes: attributes,
	})
	if err != nil {
		// CreateQueue is idempotent for identical attributes and fails only
		// when they differ, which is a confusing thing to read as a create
		// error. Say what it means.
		var exists *types.QueueNameExists
		if errors.As(err, &exists) {
			return fmt.Errorf(
				"a queue named %q already exists with different settings; edit it instead: %w",
				name, err)
		}
		return err
	}
	c.rememberURL(name, aws.ToString(out.QueueUrl))
	return nil
}

/*
 * UpdateDestination changes a queue's settings.
 *
 * Everything on the form is settable except what the name decides. A queue
 * cannot become FIFO or stop being FIFO - SetQueueAttributes answers "Unknown
 * Attribute FifoQueue" - so that is refused here, where the message can say
 * the queue has to be recreated instead.
 */
func (c *Conn) UpdateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	url, err := c.queueURL(ctx, spec.Ref.Name)
	if err != nil {
		return err
	}
	if wanted, given := spec.Attributes[AttrFIFO], isFIFO(spec.Ref.Name); given != (wanted == "true") {
		return errors.New(
			"whether a queue is FIFO is fixed at creation and spelled in its name; " +
				"create a new queue and move the messages instead")
	}

	attributes, err := c.attributesOf(ctx, spec)
	if err != nil {
		return err
	}
	if len(attributes) == 0 {
		return errors.New("nothing to change")
	}
	_, err = c.client.SetQueueAttributes(ctx, &awssqs.SetQueueAttributesInput{
		QueueUrl:   aws.String(url),
		Attributes: attributes,
	})
	return err
}

/*
 * RemoveDestination deletes a queue and everything in it.
 *
 * There is no undo and no confirmation from the service beyond the call
 * returning: SQS deletes asynchronously and keeps the name unusable for 60
 * seconds afterwards, so a queue recreated straight away is refused. The
 * cached URL goes with it, because the next call under this name has to ask
 * again rather than address a queue that is on its way out.
 */
func (c *Conn) RemoveDestination(ctx context.Context, ref model.DestinationRef) error {
	if err := c.live(); err != nil {
		return err
	}
	url, err := c.queueURL(ctx, ref.Name)
	if err != nil {
		return err
	}
	_, err = c.client.DeleteQueue(ctx, &awssqs.DeleteQueueInput{QueueUrl: aws.String(url)})
	c.forgetURL(ref.Name)
	return err
}

/*
 * attributesOf turns a spec into the attribute map SQS takes.
 *
 * Only the keys the spec carries are sent. SetQueueAttributes replaces exactly
 * what it is given and leaves the rest alone, so an omitted key means "keep
 * it" rather than "reset it" - which is what lets a form edit one setting
 * without reading and rewriting the others.
 *
 * The redrive policy is the exception: it names another queue, and SQS wants
 * that queue's ARN. The name is what a user picks, so it is resolved here.
 */
func (c *Conn) attributesOf(ctx context.Context, spec model.DestinationSpec) (map[string]string, error) {
	attributes := map[string]string{}
	for key, name := range settableAttributes {
		if value, given := spec.Attributes[key]; given && strings.TrimSpace(value) != "" {
			attributes[name] = strings.TrimSpace(value)
		}
	}

	target := strings.TrimSpace(spec.Attributes[AttrDeadLetterQueue])
	if target == "" {
		return attributes, nil
	}
	arn, err := c.queueARN(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("dead-letter queue %q: %w", target, err)
	}
	maxReceives := strings.TrimSpace(spec.Attributes[AttrMaxReceiveCount])
	if maxReceives == "" {
		maxReceives = "5"
	}
	if _, err := strconv.Atoi(maxReceives); err != nil {
		return nil, fmt.Errorf("the receive limit before a message is dead-lettered has to be a number, not %q", maxReceives)
	}
	// Quoted, which is the shape the service documents and returns.
	policy, err := json.Marshal(redrivePolicy{
		DeadLetterTargetArn: arn,
		MaxReceiveCount:     json.RawMessage(strconv.Quote(maxReceives)),
	})
	if err != nil {
		return nil, err
	}
	attributes["RedrivePolicy"] = string(policy)
	return attributes, nil
}

// queueARN reads one queue's ARN, which is what every cross-queue setting
// names it by. Nothing derives it from the URL: the account id is in both, but
// the partition - aws, aws-cn, aws-us-gov - is in neither.
func (c *Conn) queueARN(ctx context.Context, name string) (string, error) {
	url, err := c.queueURL(ctx, name)
	if err != nil {
		return "", err
	}
	out, err := c.client.GetQueueAttributes(ctx, &awssqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(url),
		AttributeNames: []types.QueueAttributeName{"QueueArn"},
	})
	if err != nil {
		return "", err
	}
	arn := out.Attributes["QueueArn"]
	if arn == "" {
		return "", fmt.Errorf("%s reported no ARN", name)
	}
	return arn, nil
}
