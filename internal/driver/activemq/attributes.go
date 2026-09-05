package activemq

// The attribute keys this driver puts on a canonical model.
//
// A contract between this package and frontend/src/mq/activemq/*.ts, not part
// of the shared vocabulary: another family's "durable" means whatever that
// family's driver decides.
//
// Most are filled by one product only, because the two report different
// things and inventing the missing half would be worse than leaving it out -
// a board that reads null draws nothing, and a board that reads a zero draws
// a figure that is not true. Which product answered is on every row so the
// board can say why a column is empty rather than looking broken.
const (
	// AttrProduct is "classic" or "artemis". On every row, because it is what
	// lets one board explain two brokers.
	AttrProduct = "product"
	// AttrKind is "queue" or "topic" - the JMS distinction, which both
	// products keep and store differently.
	AttrKind = "kind"

	AttrConsumerCount = "consumerCount"
	AttrProducerCount = "producerCount"
	AttrEnqueueCount  = "enqueueCount"
	AttrDequeueCount  = "dequeueCount"
	AttrDispatchCount = "dispatchCount"
	AttrExpiredCount  = "expiredCount"
	AttrInFlightCount = "inFlightCount"
	// AttrScheduledCount is messages accepted and waiting for their delivery
	// time. Both products have delayed delivery, and a queue that looks empty
	// while holding a hundred scheduled messages is the confusing case.
	AttrScheduledCount = "scheduledCount"

	AttrMemoryUsage   = "memoryUsage"
	AttrMemoryPercent = "memoryPercent"
	AttrMessageSize   = "averageMessageSize"

	AttrPaused    = "paused"
	AttrDurable   = "durable"
	AttrInternal  = "internal"
	AttrTemporary = "temporary"
	// AttrAutoCreated marks a destination the broker made because something
	// addressed it, rather than one somebody declared. Artemis only; Classic
	// auto-creates too and does not say so afterwards.
	AttrAutoCreated = "autoCreated"

	// AttrDeadLetter and AttrExpiry name where this destination's undeliverable
	// and expired messages go. Artemis reports them per queue; on Classic they
	// are a broker-wide policy the destination does not carry.
	AttrDeadLetter = "deadLetterAddress"
	AttrExpiry     = "expiryAddress"
	// AttrIsDeadLetter marks the destination that receives them.
	AttrIsDeadLetter = "isDeadLetter"

	// AttrRoutingTypes and AttrAddress are Artemis's two-level model surfacing:
	// an address routes and a queue stores, and for an anycast queue the two
	// names are the same string.
	AttrRoutingTypes = "routingTypes"
	AttrAddress      = "address"
	AttrFilter       = "filter"
	// AttrQueueCount is how many queues sit under an Artemis address, which
	// for a multicast address is how many durable subscriptions it has.
	AttrQueueCount = "queueCount"

	// AttrBrowseCap is the number of messages a browse will return at most,
	// where the product has such a limit. Present on Classic and absent on
	// Artemis, which pages properly.
	AttrBrowseCap = "browseCap"
)
