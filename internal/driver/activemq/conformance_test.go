package activemq

import (
	"testing"

	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/model"
)

// offlineConn is a connection with the family's declared capabilities and no
// broker behind it. Conformance is a question about the type, not about a
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
 * What ActiveMQ has no concept of, and why.
 *
 * Every entry is a capability another family in this app declares and this one
 * must not, and each reason is about JMS rather than about how far the driver
 * has got. Without this list the cheapest way to add a family is to copy a
 * neighbour's capability set, and the result is a sidebar full of pages that
 * open onto nothing.
 *
 * This is also the list phase 10 exists to produce: the roadmap put ActiveMQ
 * before NSQ to find out whether JMS semantics fit the canonical pages, and
 * the answer is written here - they fit everywhere except where the canonical
 * pages assume a log.
 */
func TestConnDeclaresNoConceptActiveMQDoesNotHave(t *testing.T) {
	absent := []struct {
		capability model.Capability
		because    string
	}{
		{
			model.CapPartitions,
			"a JMS destination is one ordered thing with no shards. Nothing splits a " +
				"queue across brokers in a way a consumer addresses, so there is no per-" +
				"partition column set to fill.",
		},
		{
			model.CapOffsetReset,
			"JMS has no offsets at all. A message is acknowledged and gone, or it is " +
				"not; there is no stored read position on either side to move.",
		},
		{
			model.CapSubscriptionPosition,
			"same reason. A durable subscriber's backlog is the messages still held " +
				"for it, not a cursor into a log that survives them.",
		},
		{
			model.CapQueueOffset,
			"and with no partitions there is not even a per-partition position to write.",
		},
		{
			model.CapOffsetClone,
			"there is no position to copy from one subscription onto another.",
		},
		{
			model.CapStreamTrim,
			"a JMS destination keeps no log to trim. Emptying one is a purge, which " +
				"names an amount to remove rather than a bound to keep, and that is a " +
				"different capability this driver does declare.",
		},
		{
			model.CapPendingEntries,
			"an in-flight message is held by the broker for a consumer and is not " +
				"enumerable: JMX reports how many are dispatched and awaiting " +
				"acknowledgement, never which ones or to whom.",
		},
		{
			model.CapPendingAdmin,
			"and with no list to read there is nothing to acknowledge or reassign.",
		},
		{
			model.CapDirectory,
			"there is no discovery tier. Brokers find each other through network " +
				"connectors they each declare, so listing them again under another " +
				"heading would be the cluster board twice.",
		},
		{
			model.CapTransactions,
			"JMS transactions are a session's, not a broker-tracked object with an " +
				"identity a page could list and a state it could be stuck in.",
		},
		{
			model.CapQuotaList,
			"nothing is throttled per client identity. What limits exist are memory " +
				"and disk on the broker, which the cluster board reports.",
		},
		{
			model.CapDestinationUpdate,
			"neither product reconfigures a destination after it exists. Classic keeps " +
				"what would be edited in activemq.xml, as a policy entry matched by name " +
				"rather than as a property of the destination. Artemis has updateQueue, " +
				"and what it changes - maximum consumers, purge-on-no-consumers, group " +
				"buckets - is Artemis vocabulary rather than anything the canonical spec " +
				"carries, so the page would offer an empty form.",
		},
		{
			model.CapAccessControl,
			"authentication and authorisation live in a JAAS realm and a set of " +
				"authorisation entries, both configured in XML and read at startup. JMX " +
				"offers no operation that creates a user or a permission, so the only " +
				"thing to draw would be a page of permanently disabled buttons.",
		},
		{
			model.CapAccessDirectory,
			"same file, same reason: changing it means editing the broker's " +
				"configuration and restarting, which is not a request a client makes.",
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

	if descriptor.Kind != model.KindActiveMQ {
		t.Errorf("kind = %q, want activemq", descriptor.Kind)
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

	// The credential half of the form has to be secrets. A password stored as
	// an option is written to disk in the clear and sent back to the renderer.
	for _, field := range descriptor.Form {
		if field.Type == model.FieldPassword && field.Target != model.TargetSecret {
			t.Errorf("form field %q holds a password but is not a secret", field.Key)
		}
	}

	// Every mechanism the form offers has to be one configOf reads, or the
	// user picks an option that quietly authenticates as nobody.
	handled := map[string]bool{
		string(model.AuthNone):  true,
		string(model.AuthPlain): true,
	}
	for _, field := range descriptor.Form {
		if field.Target != model.TargetAuth {
			continue
		}
		for _, option := range field.Options {
			if !handled[option.Value] {
				t.Errorf("the form offers the %q mechanism and configOf does not read it", option.Value)
			}
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
	for _, reason := range []string{amqpAbsent, amqpUnreachable, amqpForbidden, browseCapped} {
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
	if len(reason) < len("mq.activemq.") || reason[:len("mq.activemq.")] != "mq.activemq." {
		return false
	}
	for _, r := range reason {
		if r == ' ' {
			return false
		}
	}
	return true
}
