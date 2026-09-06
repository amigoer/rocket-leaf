package ibmmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Destinations, read from two interfaces because the queue manager has two
 * kinds of them and the REST API only describes one.
 *
 * A queue is a resource: /admin/qmgr/{qmgr}/queue answers with every queue,
 * its configuration and its runtime status in one request, and this driver
 * names the attributes it wants rather than asking for all of them - a bare
 * attributes=* returns some ninety fields per queue and the listing page reads
 * none of them.
 *
 * A topic is not a resource. There is no /topic endpoint at any version of the
 * API, so topics come from MQSC through the same server. They are on the same
 * page as queues rather than a page of their own because the canonical model
 * has one destination list and both are destinations here: an application
 * opens a queue by name and a topic by name, and which of the two it opened is
 * an attribute rather than a different concept.
 */

// Attribute keys this driver writes into model.Destination.Attributes.
//
// A contract between this package and frontend/src/mq/ibmmq/destinations.ts,
// not part of the shared vocabulary. Another family's "kind" means whatever
// that family's driver decided it means.
const (
	AttrKind        = "kind"
	AttrQueueType   = "queueType"
	AttrDescription = "description"

	AttrMaxDepth         = "maxDepth"
	AttrMaxMessageLength = "maxMessageLength"
	AttrInhibitGet       = "inhibitGet"
	AttrInhibitPut       = "inhibitPut"
	AttrTransmission     = "transmissionQueue"

	// AttrBackoutQueue and AttrBackoutThreshold are how a queue says where its
	// poison messages go. They are read on the dead-letter page too: this is
	// the pointer that makes an ordinary queue a dead-letter queue here.
	AttrBackoutQueue     = "backoutQueue"
	AttrBackoutThreshold = "backoutThreshold"

	AttrCluster     = "cluster"
	AttrOpenInput   = "openInput"
	AttrOpenOutput  = "openOutput"
	AttrOldestAge   = "oldestMessageAgeSec"
	AttrLastPut     = "lastPut"
	AttrLastGet     = "lastGet"
	AttrUncommitted = "uncommitted"
	AttrAltered     = "altered"

	// AttrDeadLetter marks the one queue the queue manager itself names. It is
	// an ordinary queue in every other respect, which is exactly why it has to
	// be marked: a depth on it means something quite different.
	AttrDeadLetter = "deadLetterQueue"

	AttrTopicString = "topicString"
	AttrTopicType   = "topicType"
)

// The two values AttrKind takes. A destination is one or the other, and every
// board that draws a row branches on it.
const (
	KindQueue = "queue"
	KindTopic = "topic"
)

// queueAttributes and queueStatus are the fixed read sets for the listing.
//
// Fixed rather than "*": a queue's full description is around ninety fields
// including several the queue manager computes, and this page reads eighteen.
// The status half is separate because the API keeps it separate - a queue's
// configuration is what it was defined as, and its status is what it is doing.
var (
	queueAttributes = []string{
		"general.description",
		"general.inhibitGet",
		"general.inhibitPut",
		"general.isTransmissionQueue",
		"storage.maximumDepth",
		"storage.maximumMessageLength",
		"extended.backoutRequeueQueueName",
		"extended.backoutThreshold",
		"cluster.name",
		"timestamps.altered",
	}
	queueStatus = []string{
		"status.currentDepth",
		"status.openInputCount",
		"status.openOutputCount",
		"status.oldestMessageAge",
		"status.lastPut",
		"status.lastGet",
		"status.uncommittedMessages",
	}
)

// queueListing is the shape /admin/qmgr/{qmgr}/queue answers with.
type queueListing struct {
	Queue []queueEntry `json:"queue"`
}

type queueEntry struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	General struct {
		Description         string `json:"description"`
		InhibitGet          bool   `json:"inhibitGet"`
		InhibitPut          bool   `json:"inhibitPut"`
		IsTransmissionQueue bool   `json:"isTransmissionQueue"`
	} `json:"general"`
	Storage struct {
		MaximumDepth         int64 `json:"maximumDepth"`
		MaximumMessageLength int64 `json:"maximumMessageLength"`
	} `json:"storage"`
	Extended struct {
		BackoutRequeueQueueName string `json:"backoutRequeueQueueName"`
		BackoutThreshold        int64  `json:"backoutThreshold"`
	} `json:"extended"`
	Cluster struct {
		Name string `json:"name"`
	} `json:"cluster"`
	Timestamps struct {
		Altered string `json:"altered"`
	} `json:"timestamps"`

	// Status is a pointer because only a local queue has one. An alias, a
	// remote definition and a model queue hold nothing, and reporting a zero
	// depth for them would be inventing a figure rather than omitting it.
	Status *queueStatusEntry `json:"status"`
}

type queueStatusEntry struct {
	CurrentDepth        int64  `json:"currentDepth"`
	OpenInputCount      int    `json:"openInputCount"`
	OpenOutputCount     int    `json:"openOutputCount"`
	OldestMessageAge    int64  `json:"oldestMessageAge"`
	LastPut             string `json:"lastPut"`
	LastGet             string `json:"lastGet"`
	UncommittedMessages int64  `json:"uncommittedMessages"`
}

// ListDestinations enumerates the queue manager's queues and topics.
func (c *Conn) ListDestinations(ctx context.Context, filter model.DestinationFilter) ([]*model.Destination, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	deadLetter, err := c.deadLetterQueueName(ctx)
	if err != nil {
		return nil, err
	}

	queues, err := c.listQueues(ctx, filter, deadLetter)
	if err != nil {
		return nil, err
	}
	topics, err := c.listTopics(ctx, filter)
	if err != nil {
		return nil, err
	}

	destinations := append(queues, topics...)
	sort.Slice(destinations, func(i, j int) bool {
		return destinations[i].Ref.Name < destinations[j].Ref.Name
	})
	for index, destination := range destinations {
		destination.ID = index + 1
	}
	return destinations, nil
}

// DestinationDetail re-reads one destination.
//
// It goes back through the listing rather than addressing the object, because
// a name alone does not say whether it is a queue or a topic and asking the
// wrong endpoint would report a topic as missing.
func (c *Conn) DestinationDetail(ctx context.Context, ref model.DestinationRef) (*model.Destination, error) {
	found, err := c.ListDestinations(ctx, model.DestinationFilter{IncludeInternal: true})
	if err != nil {
		return nil, err
	}
	for _, destination := range found {
		if destination.Ref.Name == ref.Name {
			return destination, nil
		}
	}
	return nil, fmt.Errorf("%s has no queue or topic named %q", c.qmgr, ref.Name)
}

// listQueues reads every queue in one request.
func (c *Conn) listQueues(
	ctx context.Context, filter model.DestinationFilter, deadLetter string,
) ([]*model.Destination, error) {
	// type=all rather than the default, which returns local queues only. An
	// alias and a remote definition are names an application opens and gets a
	// queue from, so leaving them out would hide half the topology from a
	// reader trying to work out where a message went.
	path := fmt.Sprintf("/qmgr/%s/queue?type=all&attributes=%s&status=%s",
		c.qmgr, strings.Join(queueAttributes, ","), strings.Join(queueStatus, ","))

	var listing queueListing
	if err := c.rest.adminGet(ctx, path, &listing); err != nil {
		return nil, err
	}

	destinations := make([]*model.Destination, 0, len(listing.Queue))
	for _, entry := range listing.Queue {
		if !filter.IncludeInternal && isInternal(entry.Name) {
			continue
		}
		destinations = append(destinations, queueDestination(entry, deadLetter))
	}
	return destinations, nil
}

func queueDestination(entry queueEntry, deadLetter string) *model.Destination {
	attributes := map[string]string{
		AttrKind:             KindQueue,
		AttrQueueType:        entry.Type,
		AttrDescription:      entry.General.Description,
		AttrMaxDepth:         strconv.FormatInt(entry.Storage.MaximumDepth, 10),
		AttrMaxMessageLength: strconv.FormatInt(entry.Storage.MaximumMessageLength, 10),
		AttrInhibitGet:       strconv.FormatBool(entry.General.InhibitGet),
		AttrInhibitPut:       strconv.FormatBool(entry.General.InhibitPut),
		AttrTransmission:     strconv.FormatBool(entry.General.IsTransmissionQueue),
		AttrBackoutQueue:     entry.Extended.BackoutRequeueQueueName,
		AttrBackoutThreshold: strconv.FormatInt(entry.Extended.BackoutThreshold, 10),
		AttrCluster:          entry.Cluster.Name,
		AttrAltered:          entry.Timestamps.Altered,
	}
	if deadLetter != "" && entry.Name == deadLetter {
		attributes[AttrDeadLetter] = "true"
	}

	destination := &model.Destination{
		Ref: model.DestinationRef{Name: entry.Name},
		// IBM MQ divides nothing. A queue is one store on one queue manager,
		// and a cluster queue is several queues that share a name rather than
		// one queue in parts - so a partition count would be a number with no
		// meaning behind it.
		Partitions: model.UnknownMetric,
		// No rate anywhere. The queue manager reports what a queue holds and
		// when it was last touched; how fast it is moving is a statistics
		// message on a system queue rather than a figure a listing can read.
		RateIn:      model.UnknownMetric,
		RateOut:     model.UnknownMetric,
		Depth:       model.UnknownMetric,
		Subscribers: model.UnknownMetric,
		LastUpdated: entry.Timestamps.Altered,
		Attributes:  attributes,
	}

	if entry.Status != nil {
		destination.Depth = entry.Status.CurrentDepth
		// Applications holding the queue open for input, which is the closest
		// thing MQ has to a consumer count: nothing subscribes to a queue, and
		// what a reader wants to know is whether anything is draining it.
		destination.Subscribers = entry.Status.OpenInputCount
		attributes[AttrOpenInput] = strconv.Itoa(entry.Status.OpenInputCount)
		attributes[AttrOpenOutput] = strconv.Itoa(entry.Status.OpenOutputCount)
		attributes[AttrLastPut] = entry.Status.LastPut
		attributes[AttrLastGet] = entry.Status.LastGet
		attributes[AttrUncommitted] = strconv.FormatInt(entry.Status.UncommittedMessages, 10)
		// -1 is the queue manager's own "no message on it", which is not an
		// age of minus one second.
		if entry.Status.OldestMessageAge >= 0 {
			attributes[AttrOldestAge] = strconv.FormatInt(entry.Status.OldestMessageAge, 10)
		}
	}
	return destination
}

/*
 * listTopics reads the topic objects, and counts what is subscribed to each.
 *
 * Two MQSC calls rather than one per topic. The subscription list is read once
 * and inverted here, because DISPLAY TPSTATUS answers for one topic string at
 * a time and a queue manager with forty topics would otherwise cost forty
 * round trips to fill one column.
 *
 * A topic object is not a topic string, and the difference matters on this
 * page: applications publish to a string, and the object is the place its
 * settings are attached. Both are carried - the name is the object, because
 * that is what an administrator manages and deletes, and the string is an
 * attribute, because that is what a publisher names.
 */
func (c *Conn) listTopics(ctx context.Context, filter model.DestinationFilter) ([]*model.Destination, error) {
	objects, err := c.display(ctx, "topic", "*", "topicstr", "descr", "type", "altdate", "alttime")
	if err != nil {
		return nil, err
	}

	subscribers, err := c.subscribersByTopicString(ctx)
	if err != nil {
		return nil, err
	}

	destinations := make([]*model.Destination, 0, len(objects))
	for _, object := range objects {
		name := mqscString(object, "topic")
		if name == "" || (!filter.IncludeInternal && isInternal(name)) {
			continue
		}
		topicString := mqscString(object, "topicstr")
		destinations = append(destinations, &model.Destination{
			Ref:        model.DestinationRef{Name: name},
			Partitions: model.UnknownMetric,
			RateIn:     model.UnknownMetric,
			RateOut:    model.UnknownMetric,
			// A topic holds nothing. What a subscription is owed sits on the
			// queue the subscription delivers to, which is on that page.
			Depth:       model.UnknownMetric,
			Subscribers: subscribers[topicString],
			LastUpdated: mqscTimestamp(object),
			Attributes: map[string]string{
				AttrKind:        KindTopic,
				AttrTopicString: topicString,
				AttrTopicType:   strings.ToLower(mqscString(object, "type")),
				AttrDescription: mqscString(object, "descr"),
				AttrAltered:     mqscTimestamp(object),
			},
		})
	}
	return destinations, nil
}

// subscribersByTopicString counts the subscriptions resolving to each topic
// string, in one call rather than one per topic.
func (c *Conn) subscribersByTopicString(ctx context.Context) (map[string]int, error) {
	objects, err := c.display(ctx, "sub", "*", "topicstr")
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(objects))
	for _, object := range objects {
		counts[mqscString(object, "topicstr")]++
	}
	return counts, nil
}

// deadLetterQueueName is the queue the queue manager sends what it cannot
// deliver. Empty is a real answer: a queue manager with no DEADQ discards
// undeliverable messages instead, which is worth seeing rather than guessing.
func (c *Conn) deadLetterQueueName(ctx context.Context) (string, error) {
	objects, err := c.display(ctx, "qmgr", "", "deadq")
	if err != nil {
		return "", err
	}
	if len(objects) == 0 {
		return "", nil
	}
	return mqscString(objects[0], "deadq"), nil
}

/*
 * isInternal reports the objects the queue manager made for itself.
 *
 * Every one of them is under SYSTEM., which is reserved by IBM and enforced -
 * a queue manager will not let an administrator define an object there. That
 * makes the prefix a rule rather than a convention, which is why this is one
 * comparison and not a list.
 *
 * The image's own DEV.* objects are not internal: somebody's configuration
 * made them, and a reader who cannot see them would think the queue manager
 * was empty.
 */
func isInternal(name string) bool {
	return strings.HasPrefix(name, "SYSTEM.")
}

// mqscTimestamp joins the two fields MQSC splits a time across.
//
// ALTDATE and ALTTIME are separate attributes and neither is useful alone. The
// result is passed through as text rather than parsed: MQ prints the queue
// manager's own local date and clock with no zone at all, and turning that
// into an instant would be claiming an offset nobody stated.
func mqscTimestamp(object map[string]json.RawMessage) string {
	date := mqscString(object, "altdate")
	clock := strings.ReplaceAll(mqscString(object, "alttime"), ".", ":")
	switch {
	case date == "":
		return clock
	case clock == "":
		return date
	default:
		return date + " " + clock
	}
}
