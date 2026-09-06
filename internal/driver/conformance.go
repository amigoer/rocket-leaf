package driver

import (
	"fmt"

	"github.com/amigoer/mq-studio/internal/model"
)

// Go code gates on the interfaces in ports.go; the UI gates on the capability
// list a Conn declares. Nothing forces those two to agree, so every driver's
// tests call CheckConformance to make a disagreement a build failure rather
// than a control that does nothing when clicked.
type capabilityBacking struct {
	capability  model.Capability
	iface       string
	implemented func(Conn) bool
}

func backings() []capabilityBacking {
	destination := func(c Conn) bool { _, ok := c.(DestinationAdmin); return ok }
	subscription := func(c Conn) bool { _, ok := c.(SubscriptionAdmin); return ok }
	runtime := func(c Conn) bool { _, ok := c.(SubscriptionRuntime); return ok }
	progress := func(c Conn) bool { _, ok := c.(ProgressAdmin); return ok }
	cloner := func(c Conn) bool { _, ok := c.(OffsetCloner); return ok }
	queueProgress := func(c Conn) bool { _, ok := c.(QueueProgressAdmin); return ok }
	reader := func(c Conn) bool { _, ok := c.(MessageReader); return ok }
	tracker := func(c Conn) bool { _, ok := c.(MessageTracker); return ok }
	tailer := func(c Conn) bool { _, ok := c.(MessageTailer); return ok }
	liveStream := func(c Conn) bool { _, ok := c.(LiveSubscriber); return ok }
	deadLetter := func(c Conn) bool { _, ok := c.(DeadLetterReader); return ok }
	pendingReader := func(c Conn) bool { _, ok := c.(PendingEntryReader); return ok }
	pendingActions := func(c Conn) bool { _, ok := c.(PendingEntryActions); return ok }
	replayer := func(c Conn) bool { _, ok := c.(MessageReplayer); return ok }
	publisher := func(c Conn) bool { _, ok := c.(MessagePublisher); return ok }
	richPublisher := func(c Conn) bool { _, ok := c.(RichPublisher); return ok }
	entryPublisher := func(c Conn) bool { _, ok := c.(EntryPublisher); return ok }
	producers := func(c Conn) bool { _, ok := c.(ProducerInspector); return ok }
	cluster := func(c Conn) bool { _, ok := c.(ClusterAdmin); return ok }
	nodeConfig := func(c Conn) bool { _, ok := c.(ConfigInspector); return ok }
	directory := func(c Conn) bool { _, ok := c.(DirectoryAdmin); return ok }
	maintenance := func(c Conn) bool { _, ok := c.(NodeMaintenance); return ok }
	logDirs := func(c Conn) bool { _, ok := c.(LogDirInspector); return ok }
	slowLog := func(c Conn) bool { _, ok := c.(SlowLogReader); return ok }
	reassign := func(c Conn) bool { _, ok := c.(PartitionReassigner); return ok }
	quotas := func(c Conn) bool { _, ok := c.(QuotaAdmin); return ok }
	transactions := func(c Conn) bool { _, ok := c.(TransactionInspector); return ok }
	writePerm := func(c Conn) bool { _, ok := c.(WritePermissionAdmin); return ok }
	access := func(c Conn) bool { _, ok := c.(AccessAdmin); return ok }
	accessDirectory := func(c Conn) bool { _, ok := c.(AccessDirectory); return ok }
	aclUsers := func(c Conn) bool { _, ok := c.(AclUserAdmin); return ok }
	namespaces := func(c Conn) bool { _, ok := c.(NamespaceAdmin); return ok }
	namespaceLimits := func(c Conn) bool { _, ok := c.(NamespaceLimits); return ok }
	identities := func(c Conn) bool { _, ok := c.(IdentityAdmin); return ok }
	identityPerms := func(c Conn) bool { _, ok := c.(IdentityPermissions); return ok }
	policies := func(c Conn) bool { _, ok := c.(PolicyAdmin); return ok }
	parameters := func(c Conn) bool { _, ok := c.(ParameterAdmin); return ok }
	definitions := func(c Conn) bool { _, ok := c.(DefinitionsAdmin); return ok }
	replication := func(c Conn) bool { _, ok := c.(ReplicationAdmin); return ok }
	streamClients := func(c Conn) bool { _, ok := c.(StreamInspector); return ok }
	routing := func(c Conn) bool { _, ok := c.(RoutingAdmin); return ok }
	census := func(c Conn) bool { _, ok := c.(CensusReporter); return ok }
	routingAdmin := func(c Conn) bool { _, ok := c.(RoutingMutator); return ok }
	clients := func(c Conn) bool { _, ok := c.(ClientInspector); return ok }
	clientClose := func(c Conn) bool { _, ok := c.(ClientCloser); return ok }
	health := func(c Conn) bool { _, ok := c.(HealthInspector); return ok }
	dlqTopology := func(c Conn) bool { _, ok := c.(DeadLetterTopology); return ok }
	stats := func(c Conn) bool { _, ok := c.(DestinationStats); return ok }
	shards := func(c Conn) bool { _, ok := c.(ShardInspector); return ok }
	actions := func(c Conn) bool { _, ok := c.(QueueActions); return ok }
	trimmer := func(c Conn) bool { _, ok := c.(StreamTrimmer); return ok }
	position := func(c Conn) bool { _, ok := c.(StreamPositionAdmin); return ok }
	scopes := func(c Conn) bool { _, ok := c.(ScopeInspector); return ok }

	return []capabilityBacking{
		{model.CapDestinationList, "DestinationAdmin", destination},
		{model.CapDestinationCreate, "DestinationAdmin", destination},
		{model.CapDestinationUpdate, "DestinationAdmin", destination},
		{model.CapDestinationDelete, "DestinationAdmin", destination},
		{model.CapPartitions, "DestinationStats", stats},
		{model.CapShards, "ShardInspector", shards},
		{model.CapDestinationPurge, "QueueActions", actions},
		{model.CapDestinationMove, "QueueActions", actions},
		{model.CapQueueRebalance, "QueueActions", actions},
		{model.CapStreamTrim, "StreamTrimmer", trimmer},
		{model.CapReassign, "PartitionReassigner", reassign},
		{model.CapQuotaList, "QuotaAdmin", quotas},
		{model.CapQuotaAdmin, "QuotaAdmin", quotas},
		{model.CapTransactions, "TransactionInspector", transactions},

		{model.CapSubscriptionList, "SubscriptionAdmin", subscription},
		{model.CapSubscriptionCreate, "SubscriptionAdmin", subscription},
		{model.CapSubscriptionDelete, "SubscriptionAdmin", subscription},
		{model.CapSubscriptionLag, "SubscriptionAdmin", subscription},
		{model.CapOffsetReset, "ProgressAdmin", progress},
		{model.CapSubscriptionRuntime, "SubscriptionRuntime", runtime},
		{model.CapOffsetClone, "OffsetCloner", cloner},
		{model.CapQueueOffset, "QueueProgressAdmin", queueProgress},
		{model.CapSubscriptionPosition, "StreamPositionAdmin", position},

		{model.CapMessageQuery, "MessageReader", reader},
		{model.CapMessageByID, "MessageReader", reader},
		{model.CapMessageTrack, "MessageTracker", tracker},
		{model.CapMessageLiveTail, "MessageTailer", tailer},
		{model.CapLiveStream, "LiveSubscriber", liveStream},
		{model.CapDLQ, "DeadLetterReader", deadLetter},
		{model.CapPendingEntries, "PendingEntryReader", pendingReader},
		{model.CapPendingAdmin, "PendingEntryActions", pendingActions},
		{model.CapMessageResend, "DeadLetterReader", deadLetter},
		{model.CapMessageReplay, "MessageReplayer", replayer},
		{model.CapPublish, "MessagePublisher", publisher},
		{model.CapDelayedDelivery, "MessagePublisher", publisher},
		{model.CapPublishRich, "RichPublisher", richPublisher},
		{model.CapEntryPublish, "EntryPublisher", entryPublisher},
		{model.CapProducerInspect, "ProducerInspector", producers},

		{model.CapClusterTopology, "ClusterAdmin", cluster},
		{model.CapClusterMetrics, "ClusterAdmin", cluster},
		{model.CapDirectory, "DirectoryAdmin", directory},
		{model.CapNodeConfig, "ConfigInspector", nodeConfig},
		{model.CapLogDirs, "LogDirInspector", logDirs},
		{model.CapSlowLog, "SlowLogReader", slowLog},
		{model.CapNodeMaintenance, "NodeMaintenance", maintenance},
		{model.CapNodeWritePerm, "WritePermissionAdmin", writePerm},
		{model.CapAccessControl, "AccessAdmin", access},
		{model.CapAccessDirectory, "AccessDirectory", accessDirectory},
		{model.CapAclUsers, "AclUserAdmin", aclUsers},
		{model.CapNamespaceList, "NamespaceAdmin", namespaces},
		{model.CapNamespaceAdmin, "NamespaceAdmin", namespaces},
		{model.CapNamespaceLimits, "NamespaceLimits", namespaceLimits},
		{model.CapIdentityList, "IdentityAdmin", identities},
		{model.CapIdentityAdmin, "IdentityAdmin", identities},
		{model.CapIdentityPermissions, "IdentityPermissions", identityPerms},
		{model.CapPolicyList, "PolicyAdmin", policies},
		{model.CapPolicyAdmin, "PolicyAdmin", policies},
		{model.CapParameterAdmin, "ParameterAdmin", parameters},
		{model.CapDefinitionsExport, "DefinitionsAdmin", definitions},
		{model.CapDefinitionsImport, "DefinitionsAdmin", definitions},
		{model.CapReplication, "ReplicationAdmin", replication},
		{model.CapStreamClients, "StreamInspector", streamClients},
		{model.CapRouting, "RoutingAdmin", routing},
		{model.CapRoutingAdmin, "RoutingMutator", routingAdmin},
		{model.CapClusterCensus, "CensusReporter", census},
		{model.CapClientInspect, "ClientInspector", clients},
		{model.CapClientClose, "ClientCloser", clientClose},
		{model.CapClusterHealth, "HealthInspector", health},
		{model.CapDeadLetterTopology, "DeadLetterTopology", dlqTopology},
		{model.CapConnectionScope, "ScopeInspector", scopes},
	}
}

// CheckConformance reports every way a connection's declared capabilities and
// its implemented interfaces disagree. An empty result means they match.
func CheckConformance(conn Conn) []error {
	capabilities := conn.Capabilities()
	problems := make([]error, 0)

	// A capability cannot be supported and degraded at once: the UI would have
	// to choose between rendering the control and explaining its absence.
	for capability := range capabilities.Degraded {
		if capabilities.Has(capability) {
			problems = append(problems, fmt.Errorf(
				"%s: %s is listed as both supported and degraded", conn.Kind(), capability))
		}
	}

	implementedIfaces := make(map[string]bool)
	declaredIfaces := make(map[string]bool)

	for _, backing := range backings() {
		implemented := backing.implemented(conn)
		if implemented {
			implementedIfaces[backing.iface] = true
		}
		// A degraded capability counts as declared: the family has the
		// concept and the page should explain its absence, which is the whole
		// point of the middle state. Only a supported one obliges the driver
		// to implement the interface behind it.
		_, degraded := capabilities.DegradedReason(backing.capability)
		if !capabilities.Has(backing.capability) && !degraded {
			continue
		}
		declaredIfaces[backing.iface] = true
		if capabilities.Has(backing.capability) && !implemented {
			problems = append(problems, fmt.Errorf(
				"%s: declares %s but does not implement %s",
				conn.Kind(), backing.capability, backing.iface))
		}
	}

	for iface := range implementedIfaces {
		if !declaredIfaces[iface] {
			problems = append(problems, fmt.Errorf(
				"%s: implements %s but declares none of its capabilities",
				conn.Kind(), iface))
		}
	}
	return problems
}
