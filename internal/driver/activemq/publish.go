package activemq

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/amigoer/mq-studio/internal/model"
)

// Sending, which is a management operation here for the same reason browsing
// is: both products expose it on the destination's MBean, so the send console
// needs no wire client and works on a broker with every acceptor switched off.
//
// What that costs is the body's type. Classic's operation is sendTextMessage
// and sends a JMS TextMessage, full stop; Artemis's sendMessage takes a type
// and can make a BytesMessage. Neither can be handed arbitrary bytes over
// JSON, so a non-text body is what the optional AMQP tier is for.

/*
 * Delayed delivery is not offered, and finding out why took sending one.
 *
 * Both products have it: Classic reads an AMQ_SCHEDULED_DELAY property and
 * Artemis an _AMQ_SCHED_DELIVERY one. But both management operations take
 * Map<String,String> and therefore set string properties, while both
 * annotations have to be a Long - so a delayed send through JMX is accepted,
 * the property is set, and the message is delivered immediately. Confirmed
 * against 6.2.0 and 2.44.0: the depth goes up straight away and Artemis's
 * ScheduledCount stays at zero.
 *
 * A real AMQP client can set a typed property, so this is something the
 * optional tier could offer later. Until then the console does not collect a
 * delay it cannot honour.
 */

// publishBatchLimit caps one send.
//
// Each message is its own JMX call, so a large count is a large batch rather
// than a loop the broker absorbs. The limit is here rather than in the form
// because the form is not the only caller.
const publishBatchLimit = 500

// Publish sends one or more messages to a destination.
//
// The canonical request is AMQP's - an exchange, a routing key, a content type
// - and ActiveMQ has none of those. The routing key is read as the destination
// name, which is what publishing to a JMS destination means, and the exchange
// is ignored: there is no topology in between for a message to take.
func (c *Conn) Publish(ctx context.Context, published model.PublishRequest) (*model.PublishResult, error) {
	target := published.RoutingKey
	if target == "" {
		target = published.Exchange
	}
	if target == "" {
		return nil, errors.New("a send needs a destination")
	}

	count := published.Count
	if count <= 0 {
		count = 1
	}
	if count > publishBatchLimit {
		return nil, fmt.Errorf("a single send is capped at %d messages", publishBatchLimit)
	}

	// The destination has to exist. Both products auto-create on demand for a
	// client that addresses one, but the management operation is on the
	// destination's own MBean - so a send to a name nothing has created yet
	// fails as a missing MBean, which reads as a broken driver rather than as
	// a typo.
	mbean, kind, err := c.destinationMBean(ctx, model.DestinationRef{
		Namespace: published.Namespace,
		Name:      target,
	})
	if err != nil {
		return nil, err
	}
	if c.tiers.product == artemis && kind == topicKind {
		// A multicast address has no queue MBean of its own to send through.
		// Sending to one of its subscriptions would reach that subscriber
		// alone, which is not what publishing to a topic means, so this says
		// so rather than quietly fanning out to one.
		return nil, errors.New(
			"artemis sends through a queue; publish to an anycast destination, " +
				"or use the amqp tier to reach a multicast address")
	}

	headers := c.publishHeaders(published)

	// One batch rather than one call per message: Jolokia takes an array, and
	// a hundred messages would otherwise be a hundred round trips.
	calls := make([]request, 0, count)
	for range count {
		calls = append(calls, c.publishCall(mbean, published, headers))
	}
	_, errs, err := c.jolokia.batchTolerant(ctx, calls)
	if err != nil {
		return nil, err
	}

	sent, refused := 0, ""
	for _, callErr := range errs {
		if callErr == nil {
			sent++
			continue
		}
		refused = callErr.Error()
	}

	return &model.PublishResult{
		Sent: sent,
		// Not unroutable, deliberately. Unroutable means the broker took the
		// message and had nowhere to put it, which is an AMQP outcome: a JMS
		// send names the destination directly, so a failure here is the send
		// being refused rather than the message being dropped afterwards. The
		// reason carries the broker's own words either way.
		Unroutable: count - sent,
		Reason:     refused,
	}, nil
}

// publishHeaders folds the canonical request's JMS-shaped fields into the one
// map both operations take.
func (c *Conn) publishHeaders(published model.PublishRequest) map[string]string {
	headers := make(map[string]string, len(published.Headers)+6)
	for key, value := range published.Headers {
		headers[key] = value
	}

	if published.CorrelationID != "" {
		headers["JMSCorrelationID"] = published.CorrelationID
	}
	if published.ReplyTo != "" {
		headers["JMSReplyTo"] = published.ReplyTo
	}
	if published.Type != "" {
		headers["JMSType"] = published.Type
	}
	if published.AppID != "" {
		headers["JMSXAppID"] = published.AppID
	}
	if published.Priority > 0 {
		headers["JMSPriority"] = strconv.Itoa(published.Priority)
	}
	if published.Expiration != "" {
		// The canonical field is a TTL in milliseconds, which is AMQP's shape.
		// JMS carries an absolute expiry, and the broker computes it from a
		// TTL header - so this is passed through as the TTL it is.
		headers["JMSExpiration"] = published.Expiration
	}

	return headers
}

func (c *Conn) publishCall(mbean string, published model.PublishRequest, headers map[string]string) request {
	if c.tiers.product == artemis {
		// Type 3 is TEXT in Artemis's message-type numbering. Sending bytes
		// would need the AMQP tier: a body arrives here as a JSON string.
		return execOperation(mbean,
			"sendMessage(java.util.Map,int,java.lang.String,boolean,java.lang.String,java.lang.String)",
			headers, 3, published.Body, published.Persistent, c.config.username, c.config.password)
	}
	// The four-argument form, which is the only Classic one that takes both
	// headers and credentials. Persistence is a broker-side decision there:
	// sendTextMessage has no delivery-mode parameter, and the destination's
	// policy decides - so the console's persistent switch is honoured on
	// Artemis and reported as unavailable on Classic.
	return execOperation(mbean,
		"sendTextMessage(java.util.Map,java.lang.String,java.lang.String,java.lang.String)",
		headers, published.Body, c.config.username, c.config.password)
}

/*
 * SendMessage is the canonical signature, which is RocketMQ's, mapped onto
 * what JMS has:
 *
 *   - topic is the destination. Both products address one directly, so this
 *     is the whole route - there is no exchange in between.
 *   - tags become a JMSType, which is the nearest JMS header: a
 *     producer-supplied label the broker stores and a selector can match.
 *   - keys become a JMSCorrelationID, the one header both products carry that
 *     a reader would search on.
 *   - delayLevel is refused rather than read. See the note above: neither
 *     management operation can set the typed property a delay needs, so
 *     honouring the parameter would mean sending immediately and reporting a
 *     delay that did not happen.
 *
 * The rich console goes through Publish instead, because a priority, a
 * per-message expiry and arbitrary headers have nowhere to go here.
 */
func (c *Conn) SendMessage(
	ctx context.Context, topic, tags, keys, body string, delayLevel int,
) (string, error) {
	published := model.PublishRequest{
		RoutingKey:    topic,
		Body:          body,
		CorrelationID: keys,
		Type:          tags,
		Persistent:    true,
		Count:         1,
	}
	if delayLevel > 0 {
		return "", errors.New(
			"activemq cannot schedule a delayed delivery through its management api")
	}

	result, err := c.Publish(ctx, published)
	if err != nil {
		return "", err
	}
	if result.Sent == 0 {
		return "", fmt.Errorf("the broker refused the message: %s", result.Reason)
	}
	// Neither operation returns a message id that survives: Classic's
	// sendTextMessage answers with one the broker generated for its own call
	// and Artemis's sendMessage answers with a string that is not a JMSMessageID.
	// Returning either would give a caller a handle that finds nothing, so
	// this returns none and says so here.
	return "", nil
}
