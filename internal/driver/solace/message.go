package solace

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Browsing a queue, which SEMP does without taking anything.
 *
 * The read is a monitored collection under the queue -
 * /monitor/msgVpns/{vpn}/queues/{queue}/msgs - and it is genuinely
 * non-destructive: the queue's spool usage, its unacknowledged count and its
 * delivery counters are all identical afterwards, and any number of readers
 * can look at the same message. That is unusual company. RabbitMQ browses
 * through basic.get, SQS through a receive that hides what it returns, Pub/Sub
 * through a pull that delivers - and only Service Bus, until now, could look
 * without cost.
 *
 * What it will not do is hand back the message. Every field the collection
 * carries is metadata - an id, a spooled time, the attachment and content
 * sizes, the redelivery count, whether it has been delivered, whether it is
 * eligible for the dead message queue - and there is no payload anywhere in
 * SEMP, at any version. The broker's own manager shows one by opening a
 * browser flow over the messaging protocol, which is a wire client this driver
 * deliberately does not have. So the caveat on this capability is the family's
 * own: the browse takes nothing and shows no body.
 */

// Message property keys this driver fills in.
//
// A contract between this package and frontend/src/mq/solace/messages.ts, not
// part of the shared vocabulary.
const (
	PropAttachmentSize = "attachmentSize"
	PropContentSize    = "contentSize"
	PropRedelivery     = "redeliveryCount"
	PropUndelivered    = "undelivered"
	PropDmqEligible    = "dmqEligible"
	PropPartitionKey   = "partitionKey"
	PropPublisherID    = "publisherId"
	PropReplicationID  = "replicationGroupMsgId"
	PropReplication    = "replicationState"
	PropSpooledTime    = "spooledTime"
)

// defaultBrowseLimit is what a caller that named none gets, and the cap is the
// caller's rather than the broker's: SEMP will page through a queue of any
// depth, and a board that asked for all of it would hold a million rows.
const (
	defaultBrowseLimit = 100
	maxBrowseLimit     = 1000
)

// msgRow is the shape the message collection answers with. Every field on it
// is metadata; the payload is not among them at any SEMP version.
type msgRow struct {
	MsgID                 int64  `json:"msgId"`
	QueueName             string `json:"queueName"`
	MsgVpnName            string `json:"msgVpnName"`
	SpooledTime           int64  `json:"spooledTime"`
	AttachmentSize        int64  `json:"attachmentSize"`
	ContentSize           int64  `json:"contentSize"`
	RedeliveryCount       int    `json:"redeliveryCount"`
	Undelivered           bool   `json:"undelivered"`
	DmqEligible           bool   `json:"dmqEligible"`
	PartitionKey          string `json:"partitionKey"`
	PublisherID           int64  `json:"publisherId"`
	ReplicationGroupMsgID string `json:"replicationGroupMsgId"`
	ReplicationState      string `json:"replicationState"`
}

/*
 * QueryMessages browses one queue.
 *
 * The queue comes from params.Topic, which is the canonical field's name
 * rather than this family's - a Solace browse is always of an endpoint, and
 * there is nothing to browse a topic with: a topic is matched against
 * subscriptions and kept nowhere.
 *
 * The time range is applied here rather than by the broker. SEMP's message
 * collection takes a count and a cursor and no filter at all, so a range is a
 * read followed by a comparison - which is honest as long as the read is
 * bounded, and it is: the limit caps how many are fetched, and messages come
 * back oldest first, so a range that excludes everything fetched returns
 * nothing rather than reading a whole queue looking.
 */
func (c *Conn) QueryMessages(ctx context.Context, params model.MessageQueryParams) ([]*model.MessageItem, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	queue := strings.TrimSpace(params.Topic)
	if queue == "" {
		return nil, fmt.Errorf("no queue named to browse")
	}

	limit := params.MaxResults
	if limit <= 0 {
		limit = defaultBrowseLimit
	}
	if limit > maxBrowseLimit {
		limit = maxBrowseLimit
	}

	path := fmt.Sprintf("/msgVpns/%s/queues/%s/msgs?count=%d",
		segment(c.vpn), segment(queue), limit)
	var rows []msgRow
	if err := c.semp.monitorGet(ctx, path, &rows); err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("%s has no queue named %s", c.vpn, queue)
		}
		return nil, err
	}

	items := make([]*model.MessageItem, 0, len(rows))
	for _, row := range rows {
		if !withinRange(row.SpooledTime, params.StartTime, params.EndTime) {
			continue
		}
		if wanted := strings.TrimSpace(params.MessageID); wanted != "" &&
			strconv.FormatInt(row.MsgID, 10) != wanted {
			continue
		}
		items = append(items, c.messageOf(row))
	}

	// Newest first, which is the order every other family's browse arrives in
	// and the one a page opens on. SEMP hands them back oldest first.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].StoreTimestamp > items[j].StoreTimestamp
	})
	for index, item := range items {
		item.ID = index + 1
	}
	return items, nil
}

/*
 * MessageByID reads one message on a queue.
 *
 * The id is scoped to the queue rather than to the broker, which is why the
 * topic argument is required: a Solace message id is the queue's own sequence
 * number, and the same number is a different message on the next queue along.
 */
func (c *Conn) MessageByID(ctx context.Context, topic, messageID string) (*model.MessageItem, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	queue := strings.TrimSpace(topic)
	if queue == "" {
		return nil, fmt.Errorf("no queue named for message %s", messageID)
	}
	id := strings.TrimSpace(messageID)
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return nil, fmt.Errorf("%q is not a solace message id, which is a number per queue", messageID)
	}

	var row msgRow
	path := "/msgVpns/" + segment(c.vpn) + "/queues/" + segment(queue) + "/msgs/" + segment(id)
	if err := c.semp.monitorGet(ctx, path, &row); err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("%s no longer holds message %s", queue, id)
		}
		return nil, err
	}
	item := c.messageOf(row)
	item.ID = 1
	return item, nil
}

func (c *Conn) messageOf(row msgRow) *model.MessageItem {
	spooled := time.Unix(row.SpooledTime, 0)
	status := model.MessageStatus("")
	if row.Undelivered {
		status = model.MessageStatus("undelivered")
	}

	return &model.MessageItem{
		Cluster: row.MsgVpnName,
		Topic:   row.QueueName,
		// The queue's own sequence number, which is the only handle SEMP
		// offers: it is what /msgs/{id} takes and what the delete action
		// names. It is not unique across queues.
		MessageID:      strconv.FormatInt(row.MsgID, 10),
		QueueOffset:    row.MsgID,
		StoreTime:      spooled.Format(time.RFC3339),
		StoreTimestamp: row.SpooledTime * 1000,
		RetryTimes:     row.RedeliveryCount,
		Status:         status,
		// Body stays empty, and that is the interface rather than an
		// unfinished read: SEMP carries no payload at any version. The caveat
		// on CapMessageQuery says so, and the board draws the sizes instead of
		// an empty panel pretending to be a message.
		Body: "",
		Properties: map[string]string{
			PropAttachmentSize: strconv.FormatInt(row.AttachmentSize, 10),
			PropContentSize:    strconv.FormatInt(row.ContentSize, 10),
			PropRedelivery:     strconv.Itoa(row.RedeliveryCount),
			PropUndelivered:    strconv.FormatBool(row.Undelivered),
			PropDmqEligible:    strconv.FormatBool(row.DmqEligible),
			PropPartitionKey:   row.PartitionKey,
			PropPublisherID:    strconv.FormatInt(row.PublisherID, 10),
			PropReplicationID:  row.ReplicationGroupMsgID,
			PropReplication:    row.ReplicationState,
			PropSpooledTime:    strconv.FormatInt(row.SpooledTime, 10),
		},
	}
}

// withinRange applies the caller's time window to a spooled time in seconds.
// A zero bound is no bound, which is what an unfilled field on the form means.
func withinRange(spooledSec, startMs, endMs int64) bool {
	if startMs > 0 && spooledSec*1000 < startMs {
		return false
	}
	if endMs > 0 && spooledSec*1000 > endMs {
		return false
	}
	return true
}
