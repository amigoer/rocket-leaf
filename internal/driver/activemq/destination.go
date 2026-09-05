package activemq

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

// Destinations, across two products that do not agree on what one is.
//
// Classic addresses a destination directly: a queue MBean and a topic MBean,
// each with the destination's name in it, and the broker lists both. Artemis
// has two levels - an address that routes and a queue that stores - so what
// the canonical page calls a destination is an address, and whether it is a
// queue or a topic is its routing type. A multicast address's queues are not
// destinations at all: each one is a durable subscription, and they belong on
// the subscriptions page.
//
// That reduction is the whole point of doing it here. Both products reach the
// pages as one shape, and neither product's vocabulary leaks into a board.

// internalPrefixes are the destinations each broker makes for itself.
//
// Classic publishes an advisory topic per destination per event, which on a
// broker with twenty queues is several hundred topics nobody declared - they
// would bury the list. Artemis keeps its own under names it also flags with an
// Internal attribute, which is read as well; the prefixes catch what predates
// the flag.
var internalPrefixes = map[product][]string{
	classic: {"ActiveMQ.Advisory."},
	artemis: {"$sys.", "activemq.notifications"},
}

// deadLetterNames are each product's default undeliverable-message
// destination. Not internal - a user wants to see it, and the dead letter page
// is built on it - but worth marking, because a dead letter queue with a depth
// means something different from an ordinary one with the same depth.
var deadLetterNames = map[product][]string{
	classic: {"ActiveMQ.DLQ"},
	artemis: {"DLQ"},
}

// ListDestinations enumerates the broker's queues and topics.
func (c *Conn) ListDestinations(ctx context.Context, filter model.DestinationFilter) ([]*model.Destination, error) {
	if c.tiers.product == artemis {
		return c.listArtemisDestinations(ctx, filter)
	}
	return c.listClassicDestinations(ctx, filter)
}

// DestinationDetail re-reads one destination.
//
// A separate call rather than a field on the list because the list is a batch
// across every destination and the detail is allowed to be expensive: it is
// what a board asks for when a row is opened, one at a time.
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
	return nil, fmt.Errorf("no destination named %q", ref.Name)
}

// classicDestinationAttributes is the fixed read set for one Classic
// destination.
//
// Fixed rather than "read the whole MBean": a bare read returns sixty
// attributes including several that walk the message store, and the list page
// asks for this once per destination.
var classicDestinationAttributes = []string{
	"QueueSize", "ConsumerCount", "ProducerCount", "EnqueueCount", "DequeueCount",
	"DispatchCount", "ExpiredCount", "InFlightCount", "MemoryUsageByteCount",
	"MemoryPercentUsage", "AverageMessageSize", "Paused", "DLQ",
}

func (c *Conn) listClassicDestinations(ctx context.Context, filter model.DestinationFilter) ([]*model.Destination, error) {
	// The broker lists its own destinations, which beats a search: the result
	// is already split into queues and topics, so the kind comes for free
	// rather than being read back out of each ObjectName.
	values, err := c.jolokia.batch(ctx, []request{
		readAttribute(c.names.brokerMBean(), "Queues"),
		readAttribute(c.names.brokerMBean(), "Topics"),
	})
	if err != nil {
		return nil, err
	}

	type entry struct {
		name  string
		kind  destinationKind
		mbean string
	}
	var entries []entry
	for i, kind := range []destinationKind{queueKind, topicKind} {
		var refs []struct {
			ObjectName string `json:"objectName"`
		}
		if err := json.Unmarshal(values[i], &refs); err != nil {
			return nil, fmt.Errorf("the broker's %s list is not a set of object names: %w", kind, err)
		}
		for _, ref := range refs {
			_, keys, err := parseObjectName(ref.ObjectName)
			if err != nil {
				continue
			}
			name := keys["destinationName"]
			if name == "" || (!filter.IncludeInternal && isInternal(classic, name)) {
				continue
			}
			entries = append(entries, entry{name: name, kind: kind, mbean: ref.ObjectName})
		}
	}

	requests := make([]request, 0, len(entries)*len(classicDestinationAttributes))
	for _, e := range entries {
		for _, attribute := range classicDestinationAttributes {
			requests = append(requests, readAttribute(e.mbean, attribute))
		}
	}
	// Tolerant: a destination removed between the broker's list and this read
	// must not cost the others their row.
	values, _, err = c.jolokia.batchTolerant(ctx, requests)
	if err != nil {
		return nil, err
	}

	destinations := make([]*model.Destination, 0, len(entries))
	for i, e := range entries {
		read := attributeSet(classicDestinationAttributes, values[i*len(classicDestinationAttributes):])
		destinations = append(destinations, c.classicDestination(e.name, e.kind, read))
	}
	sortDestinations(destinations)
	return destinations, nil
}

func (c *Conn) classicDestination(name string, kind destinationKind, read map[string]json.RawMessage) *model.Destination {
	attributes := map[string]string{
		AttrProduct: string(classic),
		AttrKind:    string(kind),
		// Classic stops a browse at maxBrowsePageSize however deep the
		// destination is, and the limit is not readable over JMX - the
		// attribute answers 404 - so this is the documented default rather
		// than this deployment's value.
		AttrBrowseCap: strconv.Itoa(classicBrowseCap),
	}
	putInt(attributes, AttrConsumerCount, read["ConsumerCount"])
	putInt(attributes, AttrProducerCount, read["ProducerCount"])
	putInt(attributes, AttrEnqueueCount, read["EnqueueCount"])
	putInt(attributes, AttrDequeueCount, read["DequeueCount"])
	putInt(attributes, AttrDispatchCount, read["DispatchCount"])
	putInt(attributes, AttrExpiredCount, read["ExpiredCount"])
	putInt(attributes, AttrInFlightCount, read["InFlightCount"])
	putInt(attributes, AttrMemoryUsage, read["MemoryUsageByteCount"])
	putInt(attributes, AttrMemoryPercent, read["MemoryPercentUsage"])
	putInt(attributes, AttrMessageSize, read["AverageMessageSize"])
	putBool(attributes, AttrPaused, read["Paused"])
	// Two judgements, and both are needed. The broker's own DLQ flag is
	// authoritative when set - a deployment can point its dead-letter strategy
	// at any destination it likes, and only the broker knows which. But
	// Classic does not set the flag until the destination has actually
	// received a dead letter, so the default one reads false on a broker that
	// has never failed a delivery, which is exactly when somebody goes looking
	// for it.
	putBool(attributes, AttrIsDeadLetter, read["DLQ"])
	if attributes[AttrIsDeadLetter] != "true" && isDeadLetter(classic, name) {
		attributes[AttrIsDeadLetter] = "true"
	}

	return &model.Destination{
		Ref:         model.DestinationRef{Name: name},
		Partitions:  model.UnknownMetric,
		Subscribers: intOr(read["ConsumerCount"], model.UnknownMetric),
		Depth:       int64(intOr(read["QueueSize"], model.UnknownMetric)),
		// Neither product reports a rate. Both keep cumulative enqueue and
		// dequeue counters, and a rate derived from two samples of those would
		// be this app's arithmetic presented as the broker's figure - the same
		// call Kafka's driver makes, for the same reason.
		RateIn:     model.UnknownMetric,
		RateOut:    model.UnknownMetric,
		Attributes: attributes,
	}
}

// artemisAddressAttributes is the read set for one address.
var artemisAddressAttributes = []string{
	"RoutingTypes", "QueueCount", "QueueNames", "MessageCount", "AddressSize",
	"Internal", "AutoCreated", "Temporary", "Paused",
}

// artemisQueueAttributes is the read set for the queue under an anycast
// address, which is where an anycast destination's real figures live.
var artemisQueueAttributes = []string{
	"MessageCount", "ConsumerCount", "MessagesAdded", "MessagesAcknowledged",
	"MessagesExpired", "DeliveringCount", "ScheduledCount", "PersistentSize",
	"Durable", "Filter", "DeadLetterAddress", "ExpiryAddress",
}

func (c *Conn) listArtemisDestinations(ctx context.Context, filter model.DestinationFilter) ([]*model.Destination, error) {
	names, err := c.jolokia.call(ctx, readAttribute(c.names.brokerMBean(), "AddressNames"))
	if err != nil {
		return nil, err
	}
	var addresses []string
	if err := json.Unmarshal(names, &addresses); err != nil {
		return nil, fmt.Errorf("the broker's address list is not a set of names: %w", err)
	}

	wanted := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if !filter.IncludeInternal && isInternal(artemis, address) {
			continue
		}
		wanted = append(wanted, address)
	}

	requests := make([]request, 0, len(wanted)*len(artemisAddressAttributes))
	for _, address := range wanted {
		for _, attribute := range artemisAddressAttributes {
			requests = append(requests, readAttribute(c.names.artemisAddress(address), attribute))
		}
	}
	values, _, err := c.jolokia.batchTolerant(ctx, requests)
	if err != nil {
		return nil, err
	}

	// An anycast address's figures live on the queue beneath it, so those are
	// a second batch. A multicast address's queues are its subscriptions and
	// are read on the subscriptions page instead; what a topic reports here is
	// how many messages its subscribers are still owed, summed.
	type pending struct {
		address string
		kind    destinationKind
		read    map[string]json.RawMessage
		queues  []string
	}
	items := make([]pending, 0, len(wanted))
	for i, address := range wanted {
		read := attributeSet(artemisAddressAttributes, values[i*len(artemisAddressAttributes):])
		if read["RoutingTypes"] == nil {
			continue
		}
		items = append(items, pending{
			address: address,
			kind:    artemisKindOf(read["RoutingTypes"]),
			read:    read,
			queues:  stringsOf(read["QueueNames"]),
		})
	}

	requests = requests[:0]
	for _, item := range items {
		for _, queue := range queueSetFor(item.kind, item.address, item.queues) {
			for _, attribute := range artemisQueueAttributes {
				requests = append(requests, readAttribute(
					c.names.artemisQueue(item.address, queue, item.kind.routing()), attribute))
			}
		}
	}
	queueValues, _, err := c.jolokia.batchTolerant(ctx, requests)
	if err != nil {
		return nil, err
	}

	destinations := make([]*model.Destination, 0, len(items))
	cursor := 0
	for _, item := range items {
		set := queueSetFor(item.kind, item.address, item.queues)
		reads := make([]map[string]json.RawMessage, 0, len(set))
		for range set {
			reads = append(reads, attributeSet(artemisQueueAttributes, queueValues[cursor:]))
			cursor += len(artemisQueueAttributes)
		}
		destinations = append(destinations, c.artemisDestination(item.address, item.kind, item.read, reads))
	}
	sortDestinations(destinations)
	return destinations, nil
}

// queueSetFor is which queues under an address carry the destination's
// figures. For anycast that is the one named after the address; for multicast
// it is every subscription, because a topic's depth is what its subscribers
// are collectively still owed.
func queueSetFor(kind destinationKind, address string, queues []string) []string {
	if kind == topicKind {
		return queues
	}
	for _, queue := range queues {
		if queue == address {
			return []string{address}
		}
	}
	// An address declared anycast with no queue under it yet: real, empty, and
	// it must still appear rather than being dropped for having no figures.
	return nil
}

func (c *Conn) artemisDestination(address string, kind destinationKind,
	addressRead map[string]json.RawMessage, queueReads []map[string]json.RawMessage) *model.Destination {

	attributes := map[string]string{
		AttrProduct: string(artemis),
		AttrKind:    string(kind),
		AttrAddress: address,
	}
	putInt(attributes, AttrQueueCount, addressRead["QueueCount"])
	putBool(attributes, AttrInternal, addressRead["Internal"])
	putBool(attributes, AttrAutoCreated, addressRead["AutoCreated"])
	putBool(attributes, AttrTemporary, addressRead["Temporary"])
	putBool(attributes, AttrPaused, addressRead["Paused"])
	if types := stringsOf(addressRead["RoutingTypes"]); len(types) > 0 {
		attributes[AttrRoutingTypes] = strings.Join(types, ",")
	}
	if isDeadLetter(artemis, address) {
		attributes[AttrIsDeadLetter] = "true"
	}

	var depth, consumers, added, acked, expired, delivering, scheduled, size int64
	for _, read := range queueReads {
		depth += int64Or(read["MessageCount"], 0)
		consumers += int64Or(read["ConsumerCount"], 0)
		added += int64Or(read["MessagesAdded"], 0)
		acked += int64Or(read["MessagesAcknowledged"], 0)
		expired += int64Or(read["MessagesExpired"], 0)
		delivering += int64Or(read["DeliveringCount"], 0)
		scheduled += int64Or(read["ScheduledCount"], 0)
		size += int64Or(read["PersistentSize"], 0)
	}
	// The per-queue settings are only meaningful for a destination that has
	// one queue. A topic's subscriptions can each carry their own filter and
	// their own dead-letter address, and one of them is not the topic's.
	if len(queueReads) == 1 {
		putBool(attributes, AttrDurable, queueReads[0]["Durable"])
		putString(attributes, AttrFilter, queueReads[0]["Filter"])
		putString(attributes, AttrDeadLetter, queueReads[0]["DeadLetterAddress"])
		putString(attributes, AttrExpiry, queueReads[0]["ExpiryAddress"])
	}

	if len(queueReads) > 0 {
		attributes[AttrConsumerCount] = strconv.FormatInt(consumers, 10)
		attributes[AttrEnqueueCount] = strconv.FormatInt(added, 10)
		attributes[AttrDequeueCount] = strconv.FormatInt(acked, 10)
		attributes[AttrExpiredCount] = strconv.FormatInt(expired, 10)
		attributes[AttrInFlightCount] = strconv.FormatInt(delivering, 10)
		attributes[AttrScheduledCount] = strconv.FormatInt(scheduled, 10)
		attributes[AttrMemoryUsage] = strconv.FormatInt(size, 10)
	}

	subscribers := model.UnknownMetric
	if kind == topicKind {
		// A topic's subscribers are its subscriptions, which exist whether or
		// not anything is connected to them - that is what durable means.
		subscribers = intOr(addressRead["QueueCount"], model.UnknownMetric)
	} else if len(queueReads) > 0 {
		subscribers = int(consumers)
	}

	depthMetric := int64(model.UnknownMetric)
	if len(queueReads) > 0 {
		depthMetric = depth
	} else if kind == topicKind {
		depthMetric = 0
	}

	return &model.Destination{
		Ref:         model.DestinationRef{Name: address},
		Partitions:  model.UnknownMetric,
		Subscribers: subscribers,
		Depth:       depthMetric,
		RateIn:      model.UnknownMetric,
		RateOut:     model.UnknownMetric,
		Attributes:  attributes,
	}
}

func artemisKindOf(routingTypes json.RawMessage) destinationKind {
	for _, routing := range stringsOf(routingTypes) {
		if strings.EqualFold(routing, "MULTICAST") {
			return topicKind
		}
	}
	return queueKind
}

func isInternal(p product, name string) bool {
	for _, prefix := range internalPrefixes[p] {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isDeadLetter(p product, name string) bool {
	for _, known := range deadLetterNames[p] {
		if name == known {
			return true
		}
	}
	return false
}

// sortDestinations puts queues before topics and orders each by name, so a
// page that refreshes does not reshuffle under the reader. Neither broker
// promises an order.
func sortDestinations(destinations []*model.Destination) {
	sort.SliceStable(destinations, func(i, j int) bool {
		left, right := destinations[i], destinations[j]
		if left.Attributes[AttrKind] != right.Attributes[AttrKind] {
			return left.Attributes[AttrKind] == string(queueKind)
		}
		return left.Ref.Name < right.Ref.Name
	})
}

// attributeSet pairs a fixed read list with the batch results that answered
// it. A member that failed is nil, which every reader below treats as absent.
func attributeSet(attributes []string, values []json.RawMessage) map[string]json.RawMessage {
	read := make(map[string]json.RawMessage, len(attributes))
	for i, attribute := range attributes {
		if i < len(values) {
			read[attribute] = values[i]
		}
	}
	return read
}

func intOr(raw json.RawMessage, fallback int) int {
	if raw == nil {
		return fallback
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return fallback
	}
	return int(value)
}

func int64Or(raw json.RawMessage, fallback int64) int64 {
	if raw == nil {
		return fallback
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return fallback
	}
	return int64(value)
}

// putInt, putBool and putString write an attribute only when the broker
// answered. An absent key reads as null in the renderer, which is what lets a
// board leave a column blank instead of drawing a zero the broker never said.
func putInt(attributes map[string]string, key string, raw json.RawMessage) {
	if raw == nil {
		return
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return
	}
	attributes[key] = strconv.FormatInt(int64(value), 10)
}

func putBool(attributes map[string]string, key string, raw json.RawMessage) {
	if raw == nil {
		return
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return
	}
	attributes[key] = strconv.FormatBool(value)
}

func putString(attributes map[string]string, key string, raw json.RawMessage) {
	if raw == nil {
		return
	}
	var value *string
	if err := json.Unmarshal(raw, &value); err != nil || value == nil || *value == "" {
		return
	}
	attributes[key] = *value
}

func stringsOf(raw json.RawMessage) []string {
	if raw == nil {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return values
}
