package model

// Capability names one operation the UI gates on.
//
// The values cross the bridge and are matched as literals in the renderer, so
// renaming one means changing frontend/src/mq/capabilities.ts in the same
// commit.
type Capability string

const (
	CapDestinationList   Capability = "destination.list"
	CapDestinationCreate Capability = "destination.create"
	CapDestinationUpdate Capability = "destination.update"
	CapDestinationDelete Capability = "destination.delete"
	CapPartitions        Capability = "destination.partitions"

	// CapShards is a family whose destination is divided into parts that are
	// objects rather than indexes.
	//
	// Distinct from CapPartitions, and the distinction is not a shade of
	// meaning. That capability's page is built around a partition number and
	// the read range at it, which is all every family that has one reports. A
	// Kinesis shard is named, owns the slice of the hash space that decides
	// which records land on it, and is changed by being split or merged rather
	// than resized - which leaves the old shard in place, closed, still
	// holding its records and named as its children's parent. A page built for
	// a number would drop exactly that.
	CapShards Capability = "destination.shards"

	// CapChannels is a family whose clients and peers reach it only through a
	// named, configured object that exists whether or not anything is using
	// it.
	//
	// Distinct from CapClientInspect, and the distinction is not a shade of
	// meaning. That capability's page lists the transport connections open
	// right now: each appears when an application connects, disappears when it
	// goes, and is addressed by a peer address. An IBM MQ channel is a
	// definition an administrator made. It exists with nothing connected, it
	// is what decides whether a connection is allowed at all, one definition
	// can have hundreds of running instances, and a message channel can sit in
	// doubt over a batch with no client anywhere near it. A page built for
	// connections would be empty on a queue manager whose applications are all
	// idle, which is exactly when somebody is looking for why they cannot
	// connect.
	CapChannels Capability = "channel.list"

	// CapDestinationPurge empties a destination without deleting it, and
	// CapDestinationMove drains one into another. Separate capabilities
	// because they are separate buttons with very different blast radii: one
	// discards, the other relocates.
	CapDestinationPurge Capability = "destination.purge"
	CapDestinationMove  Capability = "destination.move"

	// CapQueueRebalance spreads replicated destinations' leaders back across
	// the cluster. Only a family that elects a leader per destination has it.
	CapQueueRebalance Capability = "destination.rebalance"

	// CapStreamTrim discards entries from the head of a destination that keeps
	// a log, by count or by position, and deletes named entries outright.
	//
	// Distinct from CapDestinationPurge, which empties a destination in one
	// call and is the whole of what it can do. A trim is a bound the operator
	// chooses, and emptying is one setting of it - a family with this needs no
	// separate purge, and offering both would be two controls for one command.
	CapStreamTrim Capability = "destination.trim"

	CapSubscriptionList   Capability = "subscription.list"
	CapSubscriptionCreate Capability = "subscription.create"
	CapSubscriptionDelete Capability = "subscription.delete"
	CapSubscriptionLag    Capability = "subscription.lag"
	CapOffsetReset        Capability = "subscription.resetOffset"
	// CapOffsetClone is copying one subscription's read position onto another.
	// It is not CapOffsetReset: reset moves a group in time, this hands a
	// second group the first one's exact per-queue positions.
	CapOffsetClone Capability = "subscription.cloneOffset"
	// CapQueueOffset is writing one queue's read position directly. Distinct
	// from CapOffsetReset, which moves a whole subscription to a moment in
	// time and lets the broker find each queue's position for itself.
	CapQueueOffset Capability = "subscription.queueOffset"
	// CapSubscriptionPosition is moving a subscription to a named place in the
	// log rather than to a moment in time.
	//
	// Distinct from CapOffsetReset for a reason that is not cosmetic: a stream
	// entry's id is milliseconds plus a sequence within them, so a timestamp
	// cannot pick between entries sharing a millisecond, and has no way to
	// spell "the end" at all. A family whose positions are ids needs to say
	// which id.
	CapSubscriptionPosition Capability = "subscription.position"
	// CapSubscriptionRuntime is asking a connected consumer what it is doing:
	// which queues it holds and how fast it is getting through them. Only a
	// live client can answer, so a family without client introspection - or a
	// group with nothing connected - simply has no answer.
	CapSubscriptionRuntime Capability = "subscription.runtime"

	// CapPendingEntries is a family that keeps, per subscription, a list of
	// what it has handed out and not had acknowledged.
	//
	// It answers the same page as CapDLQ and CapDeadLetterTopology and cannot
	// do it their way. A dead letter is a message that was given up on and
	// moved somewhere; a pending entry is a delivery record - an id, who holds
	// it, how long they have held it and how many times it has been tried -
	// and the entry itself never moves.
	CapPendingEntries Capability = "message.pending"
	// CapPendingAdmin is acting on that list: acknowledging entries so they
	// stop being owed, and moving them to another consumer. Separate from
	// reading it, because taking work from a consumer that is merely busy is a
	// different permission and a different mistake.
	CapPendingAdmin Capability = "message.pendingAdmin"

	CapMessageQuery  Capability = "message.query"
	CapMessageByID   Capability = "message.byId"
	CapMessageTrack  Capability = "message.track"
	CapMessageResend Capability = "message.resend"
	// CapMessageReplay is handing one message back to one connected consumer
	// and reporting what its handler returned. Distinct from CapMessageResend,
	// which puts a copy back on the retry path for whoever picks it up.
	CapMessageReplay   Capability = "message.replay"
	CapMessageLiveTail Capability = "message.liveTail"
	// CapLiveStream is following what a broker pushes rather than what it
	// stores. Distinct from CapMessageLiveTail, which is an incremental read
	// of a durable log: a family can have one and not the other, and MQTT has
	// no log to tail at all.
	CapLiveStream Capability = "message.liveStream"
	CapDLQ        Capability = "message.dlq"
	CapPublish    Capability = "message.publish"

	// CapPublishRich is a send console that can set what the family's own
	// protocol carries - an exchange and routing key, headers, and the
	// delivery guarantees - rather than only a destination and a body.
	CapPublishRich Capability = "message.publishRich"
	// CapEntryPublish is a send console for a family whose message is an
	// ordered set of named fields rather than a body.
	//
	// A third shape beside CapPublish and CapPublishRich, and it has to be:
	// the first is a topic with tags, keys and a delay level, the second is an
	// exchange with a routing key and AMQP properties, and neither has
	// anywhere to put a field list or an explicit id.
	CapEntryPublish Capability = "message.publishEntry"
	// CapProducerInspect is asking who is currently publishing. It needs a
	// producer group to ask about: the broker tracks connections per group and
	// offers no way to enumerate the groups themselves.
	CapProducerInspect Capability = "message.producerInspect"
	// CapDelayedDelivery is scheduling a message for later. RocketMQ has delay
	// levels, Kafka has nothing, RabbitMQ needs a plugin.
	CapDelayedDelivery Capability = "message.delayedDelivery"

	CapClusterTopology Capability = "cluster.topology"
	// CapDirectory is listing the discovery tier a cluster is reached
	// through. Families whose nodes find each other have no such tier and do
	// not report it.
	CapDirectory Capability = "cluster.directory"
	// CapNodeConfig is reading the effective settings of a node or of the
	// cluster's discovery tier - what they are actually running with, which is
	// not always what their config files say.
	CapNodeConfig Capability = "cluster.nodeConfig"
	// CapNodeMaintenance is running a broker's own housekeeping on demand -
	// reclaiming space the broker would otherwise get to on its own schedule.
	CapNodeMaintenance Capability = "cluster.nodeMaintenance"
	// CapNodeWritePerm is taking a node out of the write path and putting it
	// back, which is how a broker is drained before it is stopped.
	CapNodeWritePerm  Capability = "cluster.writePerm"
	CapClusterMetrics Capability = "cluster.metrics"

	// CapReassign is rewriting where a destination's replicas live. Only a
	// family whose placement is data an administrator can edit has it, and
	// only Kafka's is: the replica list is a field, and the cluster copies the
	// log to its new home in the background.
	CapReassign Capability = "destination.reassign"

	// CapTransactions is a family whose producers can write across partitions
	// atomically, and whose unfinished transactions hold readers back. Only a
	// family with that mechanism has anything to report.
	CapTransactions Capability = "transaction.list"

	// CapQuotaList and CapQuotaAdmin are limits attached to a client rather
	// than to a destination - what one user, application or address may do to
	// the cluster as a whole. Only a family that throttles by identity has
	// them.
	CapQuotaList  Capability = "quota.list"
	CapQuotaAdmin Capability = "quota.admin"

	// CapSlowLog is a broker that keeps a record of the commands that took
	// longest.
	//
	// Distinct from CapNodeConfig, which is what a node is running with: this
	// is what has actually been slow on it, and it is the first thing anyone
	// looks at when a server is fine on every other figure and still not
	// keeping up.
	CapSlowLog Capability = "cluster.slowLog"

	// CapLogDirs is a broker that reports what its partitions occupy on disk.
	// Distinct from CapNodeConfig, which is what a node is running with: this
	// is where its space has gone, and it is the only disk figure Kafka has -
	// the protocol reports no free space and no percentage anywhere.
	CapLogDirs Capability = "cluster.logDirs"

	// CapClusterCensus is a broker that keeps its own running totals - object
	// counts, queued depth and message rates for the whole cluster in one
	// answer. A family whose figures can only be assembled by walking every
	// destination does not have it.
	CapClusterCensus Capability = "cluster.census"

	// CapClientInspect is a broker that can name the transport connections and
	// channels open against it. Families that expose producers and consumers
	// but not the sessions underneath them do not have it.
	CapClientInspect Capability = "client.inspect"

	// CapClientClose disconnects a client from the broker. Separate from
	// inspecting them: a monitoring user can list every connection and close
	// none.
	CapClientClose Capability = "client.close"

	// CapClusterHealth is a broker that answers questions about its own
	// health, rather than one whose health has to be inferred from its
	// metrics.
	CapClusterHealth Capability = "cluster.health"

	// CapDeadLetterTopology is a family whose dead-letter queues are ordinary
	// queues something else points at, found by walking the topology, rather
	// than a per-group topic the broker names for you. Both answer the same
	// page; neither can answer it the other's way.
	CapDeadLetterTopology Capability = "message.dlqTopology"
	CapAccessControl      Capability = "access.control"
	// CapAccessDirectory is identity-based access control: principals the
	// broker authenticates and rules attached to a subject. Distinct from
	// CapAccessControl, which is the credential-carrying kind a broker will
	// take a write for and never read back.
	CapAccessDirectory Capability = "access.directory"

	// CapAclUsers is a broker whose access control lives entirely on the user:
	// what it may run, which keys it may touch, which channels it may
	// subscribe to.
	//
	// A third model beside CapAccessControl's credential pairs and
	// CapAccessDirectory's rules attached to a subject. Redis attaches
	// everything to the principal, including the key patterns that decide what
	// data it can reach - and a page built for either of the others would show
	// the commands and hide the half that contains the data.
	CapAclUsers Capability = "access.aclUsers"

	// CapNamespaceList and CapNamespaceAdmin are families whose namespaces are
	// objects rather than labels - a RabbitMQ virtual host holds its own
	// queues, exchanges, policies and permissions, and nothing crosses
	// between two of them.
	CapNamespaceList  Capability = "namespace.list"
	CapNamespaceAdmin Capability = "namespace.admin"
	// CapNamespaceLimits caps a namespace as a whole rather than one
	// destination inside it.
	CapNamespaceLimits Capability = "namespace.limits"

	// CapIdentityList and CapIdentityAdmin are a broker that keeps its own
	// users, as opposed to one that authenticates against a credential pair
	// stored in a config file. CapIdentityPermissions is the second half of
	// that: what a user may touch, which RabbitMQ keeps separately from what
	// it may administer.
	CapIdentityList        Capability = "identity.list"
	CapIdentityAdmin       Capability = "identity.admin"
	CapIdentityPermissions Capability = "identity.permissions"

	// CapPolicyList and CapPolicyAdmin are settings applied to destinations by
	// pattern rather than at declaration. Only a family whose destinations are
	// otherwise immutable needs them, which is what makes them RabbitMQ's.
	CapPolicyList  Capability = "policy.list"
	CapPolicyAdmin Capability = "policy.admin"
	// CapParameterAdmin reads and removes the component configuration the
	// broker stores for its plugins.
	CapParameterAdmin Capability = "parameter.admin"

	// CapDefinitions is a broker that can hand back its whole topology as one
	// document and take it back. It is the only backup some families offer of
	// anything but message data.
	CapDefinitionsExport Capability = "definitions.export"
	CapDefinitionsImport Capability = "definitions.import"

	// CapReplication is moving messages between brokers - shovels and
	// federation. It is the capability most likely to be reported as degraded
	// rather than absent: both are plugins, so a broker that could do this
	// perfectly well simply has not been asked to.
	CapReplication Capability = "replication.admin"

	// CapStreamClients is who is reading and writing a stream over a protocol
	// that is not the family's main one. Degraded rather than absent for the
	// same reason as replication: it is a plugin.
	CapStreamClients Capability = "stream.clients"
	CapRouting       Capability = "routing.exchanges"

	// CapRoutingAdmin creates and deletes exchanges and bindings. Separate
	// from reading them: a connection may list a topology it has no permission
	// to change.
	CapRoutingAdmin Capability = "routing.admin"

	// CapConnectionScope is a family whose connection carries a scope that is
	// a naming convention rather than a broker object - a RocketMQ namespace
	// is a prefix the client puts on every resource it names.
	//
	// Distinct from CapNamespaceList, whose namespaces are objects one page
	// browses. This one re-points the whole connection, every page at once,
	// which is why the shell offers the switch and no page does.
	CapConnectionScope Capability = "connection.scope"
)

// Capabilities is what one live connection can actually do.
//
// Three states reach the UI, and they must stay distinguishable: a capability
// in Supported renders normally; one in Degraded renders disabled with the
// reason; one in neither is hidden outright. Silent absence and explained
// absence look identical to a user otherwise, which makes a deliberately
// limited endpoint read as a bug.
type Capabilities struct {
	Supported []Capability `json:"supported"`

	// Degraded explains a capability the family has but this endpoint lacks.
	// A RocketMQ Proxy endpoint is a data plane only, so it reports no topic
	// listing, no cluster topology and no ACL.
	Degraded map[Capability]string `json:"degraded"`

	// Caveats annotates a capability that works but has a consequence worth
	// warning about. Browsing a RabbitMQ queue goes through basic.get, which
	// alters queue state even when the message is requeued.
	Caveats map[Capability]string `json:"caveats"`
}

// NewCapabilities builds a capability set with no degraded entries or caveats.
func NewCapabilities(supported ...Capability) Capabilities {
	return Capabilities{Supported: supported}
}

// Has reports whether the connection supports the capability.
func (c Capabilities) Has(capability Capability) bool {
	for _, supported := range c.Supported {
		if supported == capability {
			return true
		}
	}
	return false
}

// DegradedReason returns why an unsupported capability is missing here.
func (c Capabilities) DegradedReason(capability Capability) (string, bool) {
	reason, ok := c.Degraded[capability]
	return reason, ok
}

// Caveat returns the warning attached to a supported capability.
func (c Capabilities) Caveat(capability Capability) (string, bool) {
	caveat, ok := c.Caveats[capability]
	return caveat, ok
}

// WithDegraded returns a copy that reports capability as unavailable here.
// It also drops it from Supported, so the two can never disagree.
func (c Capabilities) WithDegraded(capability Capability, reason string) Capabilities {
	supported := make([]Capability, 0, len(c.Supported))
	for _, current := range c.Supported {
		if current != capability {
			supported = append(supported, current)
		}
	}
	c.Supported = supported
	c.Degraded = cloneCapabilityNotes(c.Degraded)
	c.Degraded[capability] = reason
	return c
}

// WithCaveat returns a copy that keeps capability supported but warns about it.
func (c Capabilities) WithCaveat(capability Capability, caveat string) Capabilities {
	c.Caveats = cloneCapabilityNotes(c.Caveats)
	c.Caveats[capability] = caveat
	return c
}

func cloneCapabilityNotes(notes map[Capability]string) map[Capability]string {
	cloned := make(map[Capability]string, len(notes)+1)
	for capability, note := range notes {
		cloned[capability] = note
	}
	return cloned
}
