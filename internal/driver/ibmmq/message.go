package ibmmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Browsing, which is genuinely non-destructive here.
 *
 * That has to be said out loud because it is not true of the other families
 * reached through a management API. SQS's only read is the one a consumer
 * makes, so a browse hides what it read; Pub/Sub's Pull is the same and counts
 * towards being dead-lettered. IBM MQ's messaging interface has both: DELETE
 * on the message resource consumes, and GET does not. A queue browsed here has
 * the same depth afterwards, its messages stay in order, and any number of
 * readers can look at the same one.
 *
 * It is two calls rather than one. GET .../messagelist returns the identifiers
 * of the messages on the queue, in the order the queue would deliver them, and
 * nothing else; GET .../message?messageId=<id> returns one message's body with
 * its descriptor in response headers. So a browse of twenty messages is
 * twenty-one requests, which is why the limit is a real limit rather than a
 * page size.
 *
 * # The one thing it will not do
 *
 * The mqweb server carries character data and nothing else. A message the
 * queue manager stored in any other format - a dead letter, whose payload sits
 * behind a dead-letter header; a PCF event; an application's own structure -
 * is listed by messagelist with its identifier and format, and answered with
 * HTTP 501 when opened. This driver reports those rows rather than dropping
 * them: which messages are on a queue is worth knowing even when their
 * contents cannot be shown, and a browse that silently returned fewer rows
 * than the depth would be the more confusing answer.
 */

// Message property keys this driver writes into MessageItem.Properties.
//
// A contract between this package and frontend/src/mq/ibmmq/messages.ts, not
// part of the shared vocabulary.
const (
	PropFormat        = "format"
	PropCorrelationID = "correlationId"
	PropPersistence   = "persistence"
	PropExpiry        = "expiry"
	PropReplyToQueue  = "replyToQueue"
	PropReplyToQmgr   = "replyToQueueManager"

	// PropBodyUnavailable marks a message the mqweb server would not return.
	// Its value is the format that stopped it, which is the useful half: a
	// reader seeing MQDEAD knows they are looking at a dead letter rather than
	// at a broken page.
	PropBodyUnavailable = "bodyUnavailable"
)

// browseLimit is how many messages a browse asks for when the caller named no
// number. It is a real limit rather than a page size: each message costs a
// request of its own, so a default of a thousand would be a thousand requests.
const browseLimit = 50

// maxBrowseLimit caps what a caller can ask for, for the same reason.
const maxBrowseLimit = 500

// messageListEntry is one row of GET .../messagelist.
type messageListEntry struct {
	MessageID     string `json:"messageId"`
	CorrelationID string `json:"correlationId"`
	Format        string `json:"format"`
}

type messageList struct {
	Messages []messageListEntry `json:"messages"`
}

/*
 * QueryMessages browses one queue.
 *
 * params.Topic is the queue name: the canonical shape is RocketMQ's and calls
 * every destination a topic, and an IBM MQ topic has no messages to browse -
 * a publication lives on the queues its subscriptions deliver to, which are
 * queues and are browsed here like any other.
 *
 * Everything else on MessageQueryParams is unused, and that is the family
 * rather than this driver. There is no time index: the messaging interface
 * selects by message identifier and correlation identifier and by nothing
 * else, so a start and end time would be a control that quietly did nothing.
 */
func (c *Conn) QueryMessages(ctx context.Context, params model.MessageQueryParams) ([]*model.MessageItem, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	queue := strings.TrimSpace(params.Topic)
	if queue == "" {
		return nil, errors.New("no queue name given")
	}

	if id := strings.TrimSpace(params.MessageID); id != "" {
		item, err := c.MessageByID(ctx, queue, id)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, nil
		}
		return []*model.MessageItem{item}, nil
	}

	limit := params.MaxResults
	if limit <= 0 {
		limit = browseLimit
	}
	if limit > maxBrowseLimit {
		limit = maxBrowseLimit
	}

	listed, err := c.messageList(ctx, queue, limit)
	if err != nil {
		return nil, err
	}

	items := make([]*model.MessageItem, 0, len(listed))
	for index, entry := range listed {
		item, err := c.browseOne(ctx, queue, entry)
		if err != nil {
			// A message taken by a real consumer between the listing and the
			// read is gone rather than an error: the browse holds nothing, so
			// this is the ordinary race on a queue somebody is draining.
			if errors.Is(err, errNoMessage) || notFound(err) {
				continue
			}
			return nil, err
		}
		item.ID = index + 1
		items = append(items, item)
	}
	return items, nil
}

// MessageByID browses one message by its identifier.
//
// The identifier is MQ's own 24-byte MsgId, spelled as 48 hexadecimal
// characters. It is what messagelist returns and what the message resource
// selects on, so nothing here has to be derived.
func (c *Conn) MessageByID(ctx context.Context, topic, messageID string) (*model.MessageItem, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	queue := strings.TrimSpace(topic)
	id := strings.TrimSpace(messageID)
	if queue == "" || id == "" {
		return nil, errors.New("browsing one message needs a queue and a message id")
	}
	// Twenty-four zero bytes is MQMI_NONE, which is MQ's way of spelling "no
	// selector" rather than an identifier no message has. Sent as one it
	// matches whatever is at the front of the queue, so a caller asking for a
	// message that has been consumed would silently be handed a different one.
	if isNoneIdentifier(id) {
		return nil, fmt.Errorf("%q is MQ's empty message id, which matches the first message "+
			"on the queue rather than a particular one", id)
	}

	item, err := c.browseOne(ctx, queue, messageListEntry{MessageID: id})
	if err != nil {
		if errors.Is(err, errNoMessage) || notFound(err) {
			return nil, nil
		}
		return nil, err
	}
	item.ID = 1
	return item, nil
}

// messageList asks which messages are on the queue, in delivery order.
func (c *Conn) messageList(ctx context.Context, queue string, limit int) ([]messageListEntry, error) {
	path := fmt.Sprintf("/qmgr/%s/queue/%s/messagelist?limit=%d",
		c.qmgr, url.PathEscape(queue), limit)

	body, _, err := c.rest.messagingGet(ctx, path)
	if err != nil {
		return nil, err
	}
	var listing messageList
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, fmt.Errorf("the message list for %s is not json: %w", queue, err)
	}
	return listing.Messages, nil
}

/*
 * browseOne reads one message and turns it into the canonical shape.
 *
 * A body the server refuses is not an error here. HTTP 501 means the queue
 * manager has the message and mqweb cannot decode it as text, which is the
 * ordinary state of every dead letter and every event message - so the row
 * comes back with what the listing knew, marked with the format that stopped
 * it, rather than being dropped or failing the page.
 */
func (c *Conn) browseOne(ctx context.Context, queue string, entry messageListEntry) (*model.MessageItem, error) {
	path := fmt.Sprintf("/qmgr/%s/queue/%s/message?messageId=%s",
		c.qmgr, url.PathEscape(queue), url.QueryEscape(entry.MessageID))

	item := &model.MessageItem{
		Cluster:    c.qmgr,
		Topic:      queue,
		MessageID:  entry.MessageID,
		Status:     model.MsgNormal,
		Properties: map[string]string{},
	}
	if entry.Format != "" {
		item.Properties[PropFormat] = entry.Format
	}
	if entry.CorrelationID != "" {
		item.Properties[PropCorrelationID] = entry.CorrelationID
	}

	body, headers, err := c.rest.messagingGet(ctx, path)
	if err != nil {
		if unsupportedMessage(err) {
			format := entry.Format
			if format == "" {
				format = "unknown"
			}
			item.Properties[PropBodyUnavailable] = format
			return item, nil
		}
		return nil, err
	}

	item.Body = string(body)
	readDescriptor(item, headers)
	return item, nil
}

// isNoneIdentifier reports the all-zero identifier the queue manager treats as
// "match anything". It is compared as text because that is how it arrives.
func isNoneIdentifier(id string) bool {
	return strings.Trim(id, "0") == ""
}

// The response headers the message descriptor arrives in. They are IBM's own
// names and are matched case-insensitively by net/http's canonical form.
const (
	headerMessageID     = "Ibm-Mq-Md-Messageid"
	headerCorrelationID = "Ibm-Mq-Md-Correlationid"
	headerPersistence   = "Ibm-Mq-Md-Persistence"
	headerExpiry        = "Ibm-Mq-Md-Expiry"
	headerReplyToQueue  = "Ibm-Mq-Md-Replyto"
	headerReplyToQmgr   = "Ibm-Mq-Md-Replytoqmgr"
	headerUserPrefix    = "Ibm-Mq-Usr-"
)

/*
 * readDescriptor lifts what the message descriptor carries out of the headers.
 *
 * There is no put time among them, and that is the interface rather than an
 * omission here: mqweb returns the identifiers, the persistence and the expiry
 * and does not return PutDate or PutTime at all. MessageItem.StoreTime is left
 * empty rather than filled with the time of the request, which would be this
 * app's clock dressed up as the broker's.
 */
func readDescriptor(item *model.MessageItem, headers http.Header) {
	if id := headers.Get(headerMessageID); id != "" {
		item.MessageID = id
	}
	set := func(key, header string) {
		if value := headers.Get(header); value != "" {
			item.Properties[key] = value
		}
	}
	set(PropCorrelationID, headerCorrelationID)
	set(PropPersistence, headerPersistence)
	set(PropExpiry, headerExpiry)
	set(PropReplyToQueue, headerReplyToQueue)
	set(PropReplyToQmgr, headerReplyToQmgr)

	// Anything an application set as a message property comes back under its
	// own name with this prefix, so the whole set is carried rather than a
	// list this driver would have to keep up to date.
	for name, values := range headers {
		if !strings.HasPrefix(name, headerUserPrefix) || len(values) == 0 {
			continue
		}
		item.Properties[strings.TrimPrefix(name, headerUserPrefix)] = values[0]
	}
}
