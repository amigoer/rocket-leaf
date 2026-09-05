package nsq

import (
	"testing"

	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/model"
)

// offlineConn is a connection with the family's declared capabilities and no
// cluster behind it. Conformance is a question about the type, not about a
// broker, so it must be answerable with nothing running.
func offlineConn() *Conn {
	conn := &Conn{closed: make(chan struct{})}
	conn.capabilities = model.NewCapabilities(capabilities()...)
	return conn
}

// The UI gates on the capability list and Go gates on the interfaces. Nothing
// in the language forces those to agree, so this is what turns a disagreement
// into a build failure instead of a control that does nothing when clicked.
func TestConnDeclaresOnlyWhatItImplements(t *testing.T) {
	for _, problem := range driver.CheckConformance(offlineConn()) {
		t.Error(problem)
	}
}

/*
 * What NSQ has no concept of, and why.
 *
 * Every entry is a capability another family in this app declares and this one
 * must not, and each reason is about NSQ rather than about how far the driver
 * has got. Without this list the cheapest way to add a family is to copy a
 * neighbour's capability set, and the result is a sidebar full of pages that
 * open onto nothing.
 *
 * Most of the list has one root. nsqd hands a message to a consumer and stops
 * holding it: there is no stored log behind a depth, so every page built on
 * reading messages back has nothing to read. That is what phase 12 was put on
 * the roadmap to establish, and it is written here.
 */
func TestConnDeclaresNoConceptNSQDoesNotHave(t *testing.T) {
	absent := []struct {
		capability model.Capability
		because    string
	}{
		{
			model.CapMessageQuery,
			"there is no stored history to browse. A depth is a count of what is " +
				"queued in memory or spilled to a per-channel disk file, and nsqd " +
				"offers no call that reads one back - the only way a message leaves " +
				"is being delivered to a consumer, which consumes it.",
		},
		{
			model.CapMessageByID,
			"same reason, and worse: a message id exists only on the wire between " +
				"nsqd and the consumer holding it. Nothing indexes one, so there is " +
				"no id a page could be opened on.",
		},
		{
			model.CapMessageLiveTail,
			"tailing is an incremental read of a durable log, and there is no log " +
				"here to hold a position in.",
		},
		{
			model.CapDLQ,
			"nsqd never moves a message aside. A message that is requeued past the " +
				"attempt limit is dropped, and dropped is not somewhere a page can " +
				"list - there is no dead-letter topic, per channel or otherwise.",
		},
		{
			model.CapDeadLetterTopology,
			"and nothing to walk backwards to, either: a topic has no configuration " +
				"pointing failures at another topic, so there is no topology in which " +
				"one topic is another's dead letter.",
		},
		{
			model.CapMessageResend,
			"with no stored message and no dead-letter queue, there is nothing to " +
				"hand back to a consumer.",
		},
		{
			model.CapPartitions,
			"a topic is not split. It exists once per nsqd that was asked to carry " +
				"it, and those copies are independent queues rather than shards of " +
				"one ordered thing - no consumer addresses them by number, and no " +
				"call reports a range to read.",
		},
		{
			model.CapOffsetReset,
			"a channel keeps no read position. What it holds is the messages not " +
				"yet finished, so there is no cursor to move backwards or forwards.",
		},
		{
			model.CapSubscriptionPosition,
			"same reason: a backlog is the set of undelivered messages, not a place " +
				"in a log that outlives them.",
		},
		{
			model.CapQueueOffset,
			"and with no partitions there is not even a per-partition position to write.",
		},
		{
			model.CapOffsetClone,
			"there is no position to copy from one channel onto another. A new " +
				"channel starts from what is published after it exists, which is the " +
				"only starting point NSQ has.",
		},
		{
			model.CapStreamTrim,
			"a topic keeps no log to trim. Emptying one is /topic/empty, which " +
				"discards everything rather than naming a bound to keep, and that is " +
				"the purge capability this driver does declare.",
		},
		{
			model.CapDestinationMove,
			"nothing drains one topic into another. A message enters a topic by " +
				"being published to it, and nsqd offers no call that republishes what " +
				"is already queued.",
		},
		{
			model.CapPendingEntries,
			"in-flight messages are counted, never enumerated. /stats reports how " +
				"many a channel has handed out and how many one client is holding, " +
				"and there is no call that names them or says which is which.",
		},
		{
			model.CapPendingAdmin,
			"and with no list to read there is nothing to acknowledge or reassign.",
		},
		{
			model.CapDestinationUpdate,
			"a topic has no configuration. Its name is the whole of it - retention, " +
				"size limits and disk overflow are nsqd's own flags and apply to " +
				"every topic on the daemon - so an edit form would have no field to " +
				"draw. Pausing is a state this driver exposes on its own control, not " +
				"a property being reconfigured.",
		},
		{
			model.CapQueueRebalance,
			"a topic lives on the nsqd it was created on. Spreading load means " +
				"publishing to more of them, which is a producer's decision rather " +
				"than a redistribution the cluster performs.",
		},
		{
			model.CapTransactions,
			"a publish is one message and one acknowledgement. Nothing spans " +
				"topics, so there is no transaction with an identity to list or a " +
				"state to be stuck in.",
		},
		{
			model.CapAccessControl,
			"nsqd's HTTP API authenticates nobody. Its --auth-http-address delegates " +
				"authorisation for clients arriving over the TCP protocol to a service " +
				"outside NSQ, which keeps its own users and exposes no call this app " +
				"could read or write.",
		},
		{
			model.CapAccessDirectory,
			"same reason: there is no directory of principals inside NSQ to list.",
		},
		{
			model.CapNamespaceList,
			"a topic name is flat. There is no tenant, vhost or account for one to " +
				"live inside, and nothing scopes two clusters' topics apart.",
		},
		{
			model.CapClientClose,
			"nsqd will not disconnect a client on request. The HTTP API can pause a " +
				"channel, which stops delivery to every consumer of it, and that is a " +
				"different gesture from closing one connection.",
		},
		{
			model.CapClusterCensus,
			"half of a census is rates, and nsqd reports none - it counts messages " +
				"since it started and nothing else. The counts it could fill are already " +
				"on the cluster overview, so declaring this would add a panel whose " +
				"every rate is a zero nobody measured.",
		},
		{
			model.CapClusterHealth,
			"nsqd's health is one word about itself, set to whatever error broke it " +
				"and otherwise OK. There are no separate checks, no resource alarms and " +
				"no feature flags, and the one word is already the node's status on the " +
				"cluster board.",
		},
		{
			model.CapLogDirs,
			"nsqd reports no disk figure at all - not free space, not used, not a " +
				"percentage. A topic's overflow file sits wherever --data-path points " +
				"and the daemon never looks at it.",
		},
		{
			model.CapNodeMaintenance,
			"there is no housekeeping to run. nsqd has no retention sweep to bring " +
				"forward: a message is held until a consumer takes it, and the disk " +
				"overflow shrinks as that happens.",
		},
	}

	live := offlineConn().Capabilities()
	for _, entry := range absent {
		if live.Has(entry.capability) {
			t.Errorf("declares %s, but %s", entry.capability, entry.because)
		}
		if _, degraded := live.DegradedReason(entry.capability); degraded {
			t.Errorf("degrades %s, which implies the family has it; %s",
				entry.capability, entry.because)
		}
	}
}

// The descriptor is read before anything is dialled, so it has to stand on its
// own: a form that writes into a target nothing reads, or a capability the
// connection cannot honour, would both surface as a dead control.
func TestDescriptorIsSelfConsistent(t *testing.T) {
	descriptor := New().Descriptor()

	if descriptor.Kind != model.KindNSQ {
		t.Errorf("kind = %q, want nsq", descriptor.Kind)
	}
	if descriptor.DefaultPort != defaultPort {
		t.Errorf("default port = %q, want %q", descriptor.DefaultPort, defaultPort)
	}
	if len(descriptor.Form) == 0 {
		t.Fatal("descriptor carries no connection form")
	}

	keys := make(map[string]bool, len(descriptor.Form))
	for _, field := range descriptor.Form {
		if field.Key == "" || field.LabelKey == "" {
			t.Errorf("form field is missing a key or label: %#v", field)
		}
		if keys[field.Key] {
			t.Errorf("form field %q is declared twice", field.Key)
		}
		keys[field.Key] = true
		switch field.Target {
		case model.TargetEndpoints, model.TargetOption, model.TargetSecret, model.TargetAuth:
		default:
			t.Errorf("form field %q writes into an unknown target %q", field.Key, field.Target)
		}
		if field.Type == model.FieldSelect && len(field.Options) == 0 {
			t.Errorf("form field %q is a select with no options", field.Key)
		}
	}

	for _, field := range descriptor.Form {
		if field.VisibleWhen == nil {
			continue
		}
		if !keys[field.VisibleWhen.Field] {
			t.Errorf("form field %q is shown by %q, which is not on the form",
				field.Key, field.VisibleWhen.Field)
		}
		if len(field.VisibleWhen.Equals) == 0 {
			t.Errorf("form field %q has a condition that matches nothing", field.Key)
		}
	}

	// The one family here with no credential row at all, and it has to stay
	// that way while that is the truth about the API: a field offering to
	// authenticate against an endpoint that authenticates nobody would send a
	// user looking for the account that is being refused.
	for _, field := range descriptor.Form {
		if field.Target == model.TargetSecret || field.Target == model.TargetAuth {
			t.Errorf("form field %q collects a credential, and nsqd's HTTP API takes none", field.Key)
		}
	}

	live := offlineConn().Capabilities()
	for _, capability := range descriptor.MaxCapabilities {
		if live.Has(capability) {
			continue
		}
		if reason, degraded := live.DegradedReason(capability); degraded {
			if reason == "" {
				t.Errorf("%s is degraded with no reason to show", capability)
			}
			continue
		}
		t.Errorf("descriptor promises %s but a connection neither supports nor degrades it", capability)
	}
}

// The reasons cross a language boundary as keys and are resolved by the
// sidebar at runtime, so a sentence here reaches the screen as a sentence in
// the wrong language - and the renderer's own key test cannot see them,
// because it only scans literal t("...") calls in the frontend source.
func TestDegradedReasonsAreTranslationKeys(t *testing.T) {
	for _, reason := range []string{lookupdAbsent} {
		if reason == "" {
			t.Error("a reason is empty")
			continue
		}
		if !isTranslationKey(reason) {
			t.Errorf("%q is a sentence, not an i18n key", reason)
		}
	}
}

func isTranslationKey(reason string) bool {
	const prefix = "mq.nsq."
	if len(reason) < len(prefix) || reason[:len(prefix)] != prefix {
		return false
	}
	for _, r := range reason {
		if r == ' ' {
			return false
		}
	}
	return true
}

// A connection with no nsqlookupd has to say so rather than drawing an empty
// directory board. It is the one capability this family's connection can be
// without, and the reason is a configuration a user can act on.
func TestADirectorylessConnectionExplainsItself(t *testing.T) {
	conn := &Conn{closed: make(chan struct{})}
	declared := conn.declare()

	if declared.Has(model.CapDirectory) {
		t.Error("a connection naming no nsqlookupd still claims a discovery tier")
	}
	reason, degraded := declared.DegradedReason(model.CapDirectory)
	if !degraded {
		t.Fatal("the missing discovery tier is neither supported nor explained")
	}
	if reason != lookupdAbsent {
		t.Errorf("reason = %q, want %q", reason, lookupdAbsent)
	}

	// With one configured it comes back, and nothing else moves.
	withTier := &Conn{closed: make(chan struct{}), config: clientConfig{lookupd: []string{"http://x:4161"}}}
	if !withTier.declare().Has(model.CapDirectory) {
		t.Error("a connection naming an nsqlookupd does not claim a discovery tier")
	}
}
