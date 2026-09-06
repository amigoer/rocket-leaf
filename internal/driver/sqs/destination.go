package sqs

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

// Attribute keys this driver puts on a destination.
//
// A contract between this package and frontend/src/mq/sqs/destinations.ts,
// not part of the shared vocabulary.
const (
	AttrURL             = "url"
	AttrARN             = "arn"
	AttrFIFO            = "fifo"
	AttrVisible         = "visible"
	AttrInFlight        = "inFlight"
	AttrDelayed         = "delayed"
	AttrVisibilityTimeo = "visibilityTimeoutSec"
	AttrDelaySeconds    = "delaySec"
	AttrRetentionSec    = "retentionSec"
	AttrMaxMessageBytes = "maxMessageBytes"
	AttrReceiveWaitSec  = "receiveWaitSec"
	AttrCreatedAt       = "createdAt"
	AttrModifiedAt      = "modifiedAt"
	AttrDeadLetterQueue = "deadLetterQueue"
	AttrMaxReceiveCount = "maxReceiveCount"
	AttrEncrypted       = "encrypted"
	AttrContentDedup    = "contentBasedDeduplication"
	AttrDedupScope      = "deduplicationScope"
	AttrThroughputLimit = "fifoThroughputLimit"
)

// listPageSize is how many queue URLs one ListQueues call asks for. 1000 is
// the service's own maximum, and paging is the only way past it.
const listPageSize = 1000

// attributeFanOut caps how many GetQueueAttributes calls run at once.
//
// ListQueues answers with URLs and nothing else, so every figure on the board
// is a second request per queue. Unbounded that is a thousand requests fired
// together at an API that throttles per account, and the throttling would
// arrive as a failed listing rather than a slow one.
const attributeFanOut = 16

/*
 * ListDestinations enumerates the region's queues and reads each one's
 * attributes.
 *
 * Two calls per board rather than one, and there is no way around it:
 * ListQueues answers with URLs, and every number a listing shows -
 * depth, in flight, delayed, the redrive policy - is on GetQueueAttributes,
 * which takes one queue. So the fan-out is bounded and the failures are
 * tolerated per queue: a listing that races a delete drops the row rather
 * than failing the board, which is the ordinary case in an account several
 * teams share.
 */
func (c *Conn) ListDestinations(ctx context.Context, _ model.DestinationFilter) ([]*model.Destination, error) {
	// The filter's IncludeInternal has nothing to hide. SQS has no queues of
	// its own: every name in the listing is one somebody created.
	if err := c.live(); err != nil {
		return nil, err
	}

	urls, err := c.listQueueURLs(ctx)
	if err != nil {
		return nil, err
	}

	destinations := make([]*model.Destination, len(urls))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(attributeFanOut)
	for index, url := range urls {
		group.Go(func() error {
			attributes, err := c.queueAttributes(groupCtx, url)
			if err != nil {
				// A queue that has gone since the listing is not an error the
				// board should show. Anything else is, because a throttled or
				// forbidden read would otherwise be a row silently missing.
				if notFound(err) {
					return nil
				}
				return fmt.Errorf("%s: %w", queueNameOf(url), err)
			}
			destinations[index] = destinationOf(url, attributes)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	kept := make([]*model.Destination, 0, len(destinations))
	for _, destination := range destinations {
		if destination != nil {
			c.rememberURL(destination.Ref.Name, destination.Attribute(AttrURL))
			kept = append(kept, destination)
		}
	}
	return kept, nil
}

// DestinationDetail reads one queue.
//
// Not a walk of the listing: GetQueueAttributes takes one queue, so an
// account with a thousand of them should not answer for all of them to
// describe one.
func (c *Conn) DestinationDetail(ctx context.Context, ref model.DestinationRef) (*model.Destination, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	url, err := c.queueURL(ctx, ref.Name)
	if err != nil {
		return nil, err
	}
	attributes, err := c.queueAttributes(ctx, url)
	if err != nil {
		return nil, err
	}
	return destinationOf(url, attributes), nil
}

// listQueueURLs pages through the region's queues, honouring the profile's
// prefix.
//
// The prefix is the only filter SQS offers, and it is applied by the service
// rather than here - which matters in an account holding thousands of queues
// for several teams, where the difference is a page of results against
// everything.
func (c *Conn) listQueueURLs(ctx context.Context) ([]string, error) {
	var urls []string
	var token *string
	for {
		input := &awssqs.ListQueuesInput{
			MaxResults: aws.Int32(listPageSize),
			NextToken:  token,
		}
		if c.config.prefix != "" {
			input.QueueNamePrefix = aws.String(c.config.prefix)
		}
		page, err := c.client.ListQueues(ctx, input)
		if err != nil {
			return nil, err
		}
		urls = append(urls, page.QueueUrls...)
		if page.NextToken == nil || aws.ToString(page.NextToken) == "" {
			break
		}
		token = page.NextToken
	}
	sort.Strings(urls)
	return urls, nil
}

// queueAttributes reads every attribute of one queue.
//
// "All" rather than a named list: the set differs between standard and FIFO
// queues and grows with the service, and naming them would mean a queue whose
// newest setting this app cannot see.
func (c *Conn) queueAttributes(ctx context.Context, url string) (map[string]string, error) {
	out, err := c.client.GetQueueAttributes(ctx, &awssqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(url),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameAll},
	})
	if err != nil {
		return nil, err
	}
	return out.Attributes, nil
}

/*
 * destinationOf turns one queue's attributes into a canonical destination.
 *
 * Depth is everything the queue is holding: visible, in flight and delayed
 * added together. Each is a different kind of held - available to a consumer
 * now, handed out and not yet deleted, and not due yet - so the three are
 * carried separately as well, because a queue whose whole depth is in flight
 * is a stuck consumer and one whose whole depth is visible is no consumer at
 * all.
 *
 * Every figure is approximate and says so in its own name. SQS is distributed
 * and reports what its servers last agreed on, so two reads a second apart can
 * disagree with no message having moved.
 */
func destinationOf(url string, attributes map[string]string) *model.Destination {
	name := queueNameOf(url)
	visible := numberAttr(attributes, "ApproximateNumberOfMessages")
	inFlight := numberAttr(attributes, "ApproximateNumberOfMessagesNotVisible")
	delayed := numberAttr(attributes, "ApproximateNumberOfMessagesDelayed")

	collected := map[string]string{
		AttrURL:             url,
		AttrARN:             attributes["QueueArn"],
		AttrFIFO:            strconv.FormatBool(isFIFO(name)),
		AttrVisible:         strconv.FormatInt(visible, 10),
		AttrInFlight:        strconv.FormatInt(inFlight, 10),
		AttrDelayed:         strconv.FormatInt(delayed, 10),
		AttrVisibilityTimeo: attributes["VisibilityTimeout"],
		AttrDelaySeconds:    attributes["DelaySeconds"],
		AttrRetentionSec:    attributes["MessageRetentionPeriod"],
		AttrMaxMessageBytes: attributes["MaximumMessageSize"],
		AttrReceiveWaitSec:  attributes["ReceiveMessageWaitTimeSeconds"],
		AttrCreatedAt:       attributes["CreatedTimestamp"],
		AttrModifiedAt:      attributes["LastModifiedTimestamp"],
		// Either kind of server-side encryption counts. Which key is in use is
		// a KMS question this connection is not signed to answer.
		AttrEncrypted: strconv.FormatBool(
			attributes["SqsManagedSseEnabled"] == "true" || attributes["KmsMasterKeyId"] != ""),
	}

	if target, maxReceives, ok := redriveOf(attributes["RedrivePolicy"]); ok {
		collected[AttrDeadLetterQueue] = target
		collected[AttrMaxReceiveCount] = maxReceives
	}
	// FIFO-only settings, left out on a standard queue rather than written as
	// a default it does not have.
	for key, attribute := range map[string]string{
		AttrContentDedup:    "ContentBasedDeduplication",
		AttrDedupScope:      "DeduplicationScope",
		AttrThroughputLimit: "FifoThroughputLimit",
	} {
		if value := attributes[attribute]; value != "" {
			collected[key] = value
		}
	}

	return &model.Destination{
		Ref:        model.DestinationRef{Name: name},
		Partitions: model.UnknownMetric,
		// SQS keeps no record of who reads a queue: a consumer is whoever
		// calls ReceiveMessage. Zero here would read as "nothing is consuming
		// this", which is a claim the service cannot support.
		Subscribers: model.UnknownMetric,
		Depth:       visible + inFlight + delayed,
		// No rates anywhere. SQS publishes them to CloudWatch, which is a
		// different API under a different permission, and two samples taken
		// here would be this app's arithmetic presented as the service's.
		RateIn:     model.UnknownMetric,
		RateOut:    model.UnknownMetric,
		Attributes: collected,
	}
}

// redrivePolicy is the JSON SQS stores on a queue that dead-letters.
//
// maxReceiveCount is quoted in the documented shape and unquoted in some
// answers, so it is decoded loosely rather than as a number.
type redrivePolicy struct {
	DeadLetterTargetArn string          `json:"deadLetterTargetArn"`
	MaxReceiveCount     json.RawMessage `json:"maxReceiveCount"`
}

// redriveOf reads the dead-letter target and the attempt limit off a policy.
//
// The target is an ARN, and the name is its last segment: nothing in the
// policy spells the queue's name, and resolving the ARN would be a request per
// queue for a string already inside it.
func redriveOf(raw string) (target, maxReceives string, ok bool) {
	if strings.TrimSpace(raw) == "" {
		return "", "", false
	}
	var policy redrivePolicy
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		return "", "", false
	}
	if policy.DeadLetterTargetArn == "" {
		return "", "", false
	}
	return queueNameOf(policy.DeadLetterTargetArn), looseNumber(policy.MaxReceiveCount), true
}

// looseNumber renders a JSON value that may be a number or a quoted one.
func looseNumber(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var quoted string
	if err := json.Unmarshal(raw, &quoted); err == nil {
		return quoted
	}
	return strings.TrimSpace(string(raw))
}

func numberAttr(attributes map[string]string, key string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(attributes[key]), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
