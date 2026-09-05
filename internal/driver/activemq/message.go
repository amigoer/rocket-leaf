package activemq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/amigoer/mq-studio/internal/model"
)

// Browsing, which is a management operation here rather than a consume.
//
// That is the unusual thing about this family and it is worth being explicit
// about: both products expose browse as a JMX operation on the destination, so
// reading a queue takes nothing off it and needs no wire client. RabbitMQ's
// browse goes through basic.get and alters the queue even when what it read is
// put back; ActiveMQ's does not, and the message page carries no caveat about
// it.
//
// What it does carry is Classic's limit. browse() stops at maxBrowsePageSize -
// 400 by default - however deep the destination is, and the limit is not
// readable over JMX, so a deep queue silently returns a short page. The driver
// reports that as a caveat rather than pretending the queue is 400 deep.

// Message attribute keys, on top of the shared ones.
const (
	AttrPriority     = "priority"
	AttrPersistent   = "persistent"
	AttrRedelivered  = "redelivered"
	AttrCorrelation  = "correlationId"
	AttrReplyTo      = "replyTo"
	AttrJMSType      = "jmsType"
	AttrExpiration   = "expiration"
	AttrProtocol     = "protocol"
	AttrLargeMessage = "largeMessage"
	AttrGroupID      = "groupId"
	AttrGroupSeq     = "groupSeq"
	AttrTruncated    = "truncated"
)

// browseLimit is how many messages one page asks for when the caller named no
// maximum. Classic ignores it and applies its own cap; Artemis honours it.
const browseLimit = 200

// QueryMessages browses a destination.
//
// The canonical params carry a time window and a key, which are RocketMQ's
// search terms. Neither product indexes by either: a JMS browse returns the
// head of the destination in order, and filtering is a JMS selector the broker
// evaluates. So the window and the key are applied here, over what came back,
// and the page says that is what happened rather than implying the broker
// searched.
func (c *Conn) QueryMessages(ctx context.Context, params model.MessageQueryParams) ([]*model.MessageItem, error) {
	if params.Topic == "" {
		return nil, errors.New("browsing needs a destination")
	}

	limit := params.MaxResults
	if limit <= 0 {
		limit = browseLimit
	}

	mbean, kind, err := c.destinationMBean(ctx, model.DestinationRef{
		Namespace: params.Cluster,
		Name:      params.Topic,
	})
	if err != nil {
		return nil, err
	}
	if c.tiers.product == artemis && kind == topicKind {
		// A multicast address holds nothing: the messages are in its
		// subscription queues, and which one to read is a choice rather than
		// a default. The subscriptions page is where that choice is made.
		return nil, errors.New(
			"an artemis topic holds no messages of its own; browse one of its subscriptions")
	}

	raw, err := c.browse(ctx, mbean, limit)
	if err != nil {
		return nil, err
	}

	messages := make([]*model.MessageItem, 0, len(raw))
	for _, entry := range raw {
		message := c.messageOf(params.Topic, entry)
		if !matchesQuery(message, params) {
			continue
		}
		messages = append(messages, message)
		if len(messages) >= limit {
			break
		}
	}
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].StoreTimestamp > messages[j].StoreTimestamp
	})
	return messages, nil
}

// MessageByID reads one message out of the destination it is sitting in.
//
// A scan rather than a lookup, and the same one the page already does: neither
// product indexes a destination by message id, so the only way to find one is
// to browse and compare. Kafka's key search is a scan for the same reason.
func (c *Conn) MessageByID(ctx context.Context, topic, messageID string) (*model.MessageItem, error) {
	messages, err := c.QueryMessages(ctx, model.MessageQueryParams{
		Topic:      topic,
		MaxResults: browseLimit,
	})
	if err != nil {
		return nil, err
	}
	for _, message := range messages {
		if message.MessageID == messageID {
			return message, nil
		}
	}
	return nil, fmt.Errorf("no message %q in %q within the browsable window", messageID, topic)
}

// browse returns the raw message maps, whose keys differ entirely between the
// two products - see messageOf.
func (c *Conn) browse(ctx context.Context, mbean string, limit int) ([]map[string]json.RawMessage, error) {
	var call request
	if c.tiers.product == artemis {
		// Artemis pages properly, so the limit is the broker's business
		// rather than something to trim afterwards. Page 1 is the head.
		call = execOperation(mbean, "browse(int,int)", 1, limit)
	} else {
		// Classic takes no arguments and applies maxBrowsePageSize itself.
		call = execOperation(mbean, "browse()")
	}

	value, err := c.jolokia.call(ctx, call)
	if err != nil {
		return nil, err
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(value, &entries); err != nil {
		return nil, fmt.Errorf("the browse result is not a list of messages: %w", err)
	}
	return entries, nil
}

// messageOf maps one browse entry onto the canonical model.
//
// The two products share no key at all, which is the single most surprising
// thing about this driver: Classic answers with JMS header names - JMSMessageID,
// Text, JMSTimestamp - and Artemis with its own lower-case set - messageID,
// text, timestamp - plus fields Classic has no equivalent for. Both were read
// off a real browse rather than out of documentation.
func (c *Conn) messageOf(topic string, entry map[string]json.RawMessage) *model.MessageItem {
	if c.tiers.product == artemis {
		return c.artemisMessage(topic, entry)
	}
	return c.classicMessage(topic, entry)
}

func (c *Conn) classicMessage(topic string, entry map[string]json.RawMessage) *model.MessageItem {
	timestamp := millisOf(entry["JMSTimestamp"])
	properties := jmsProperties(entry)

	attributes := map[string]string{
		AttrProduct: string(classic),
	}
	putInt(attributes, AttrPriority, entry["JMSPriority"])
	putBool(attributes, AttrRedelivered, entry["JMSRedelivered"])
	putString(attributes, AttrCorrelation, entry["JMSCorrelationID"])
	putString(attributes, AttrReplyTo, entry["JMSReplyTo"])
	putString(attributes, AttrJMSType, entry["JMSType"])
	putInt(attributes, AttrExpiration, entry["JMSExpiration"])
	putString(attributes, AttrGroupID, entry["JMSXGroupID"])
	putInt(attributes, AttrGroupSeq, entry["JMSXGroupSeq"])
	// Classic renders JMSDeliveryMode as the word rather than the JMS number,
	// so this is a string comparison and not a 2-means-persistent test. Both
	// forms are accepted because a Jolokia agent configured for raw values
	// answers with the number.
	attributes[AttrPersistent] = strconv.FormatBool(isPersistent(entry["JMSDeliveryMode"]))
	for key, value := range attributes {
		properties[key] = value
	}

	return &model.MessageItem{
		Topic:          topic,
		MessageID:      stringOr(entry["JMSMessageID"]),
		Keys:           stringOr(entry["JMSCorrelationID"]),
		QueueID:        model.UnknownMetric,
		QueueOffset:    int64(model.UnknownMetric),
		StoreTime:      timeOf(timestamp),
		StoreTimestamp: timestamp,
		Body:           stringOr(entry["Text"]),
		Properties:     properties,
	}
}

func (c *Conn) artemisMessage(topic string, entry map[string]json.RawMessage) *model.MessageItem {
	timestamp := millisOf(entry["timestamp"])
	properties := jmsProperties(entry)

	attributes := map[string]string{
		AttrProduct: string(artemis),
	}
	putInt(attributes, AttrPriority, entry["priority"])
	putBool(attributes, AttrPersistent, entry["durable"])
	putBool(attributes, AttrRedelivered, entry["redelivered"])
	putString(attributes, AttrProtocol, entry["protocol"])
	putBool(attributes, AttrLargeMessage, entry["largeMessage"])
	putInt(attributes, AttrExpiration, entry["expiration"])
	putString(attributes, AttrGroupID, entry["groupID"])
	for key, value := range attributes {
		properties[key] = value
	}

	// A large message's body is not in the browse result: Artemis stores it
	// outside the journal and browse reports the flag instead. Saying so beats
	// showing an empty body as though the message were empty.
	body := stringOr(entry["text"])
	if body == "" && attributes[AttrLargeMessage] == "true" {
		properties[AttrTruncated] = "true"
	}

	return &model.MessageItem{
		Topic:          topic,
		MessageID:      stringOr(entry["messageID"]),
		QueueID:        model.UnknownMetric,
		QueueOffset:    int64(model.UnknownMetric),
		StoreTime:      timeOf(timestamp),
		StoreTimestamp: timestamp,
		Body:           body,
		Properties:     properties,
	}
}

// jmsProperties flattens the typed property maps both products report.
//
// JMS keeps user properties by type - StringProperties, IntProperties and four
// more - so a message with one of each arrives as six maps. The canonical
// model has one, and the type is not lost: a reader who needs it can see the
// value, and inventing a "propertyTypes" side-channel for something no board
// draws would be shipping what nothing reads.
func jmsProperties(entry map[string]json.RawMessage) map[string]string {
	properties := make(map[string]string, 8)
	for _, key := range []string{
		"StringProperties", "IntProperties", "LongProperties", "DoubleProperties",
		"FloatProperties", "ShortProperties", "ByteProperties", "BooleanProperties",
	} {
		raw, ok := entry[key]
		if !ok || raw == nil {
			continue
		}
		var typed map[string]any
		if err := json.Unmarshal(raw, &typed); err != nil {
			continue
		}
		for name, value := range typed {
			properties[name] = fmt.Sprint(value)
		}
	}
	return properties
}

// matchesQuery applies the canonical search terms here, because neither broker
// can. A JMS browse returns the head of the destination in order; there is no
// index on time and none on a key.
func matchesQuery(message *model.MessageItem, params model.MessageQueryParams) bool {
	if params.MessageID != "" && message.MessageID != params.MessageID {
		return false
	}
	if params.StartTime > 0 && message.StoreTimestamp < params.StartTime {
		return false
	}
	if params.EndTime > 0 && message.StoreTimestamp > params.EndTime {
		return false
	}
	if key := params.MessageKey; key != "" {
		if !strings.Contains(message.Body, key) && !strings.Contains(message.Keys, key) {
			return false
		}
	}
	return true
}

// millisOf reads a timestamp the two products render differently.
//
// Artemis answers with epoch milliseconds. Classic answers with an ISO-8601
// string - "2026-09-05T16:34:32.882Z" - which unmarshals into no number at
// all, so a driver expecting one gets zero and every message reads as having
// no time. That is what this exists to not do.
func millisOf(raw json.RawMessage) int64 {
	if raw == nil {
		return 0
	}
	var epoch float64
	if err := json.Unmarshal(raw, &epoch); err == nil {
		return int64(epoch)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil || text == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z0700"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
}

// isPersistent reads JMSDeliveryMode in either of the forms Jolokia renders.
func isPersistent(raw json.RawMessage) bool {
	if raw == nil {
		// Absent means the broker did not say, and JMS defaults to
		// persistent - the safer of the two to report.
		return true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return !strings.Contains(strings.ToUpper(text), "NON")
	}
	var mode float64
	if err := json.Unmarshal(raw, &mode); err == nil {
		// 2 is PERSISTENT in JMS, 1 is NON_PERSISTENT.
		return int(mode) == 2
	}
	return true
}

func timeOf(milliseconds int64) string {
	if milliseconds <= 0 {
		return ""
	}
	return time.UnixMilli(milliseconds).Format(time.RFC3339)
}
