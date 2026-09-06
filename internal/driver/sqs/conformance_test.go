package sqs

import (
	"strings"
	"testing"

	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/model"
)

// offlineConn is a connection with the family's declared capabilities and no
// account behind it. Conformance is a question about the type, not about a
// service, so it must be answerable with nothing running.
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
 * What SQS has no concept of, and why.
 *
 * Every entry is a capability another family in this app declares and this one
 * must not, and each reason is about SQS rather than about how far the driver
 * has got. Without this list the cheapest way to add a family is to copy a
 * neighbour's capability set, and the result is a sidebar full of pages that
 * open onto nothing.
 *
 * Two roots cover nearly all of it. SQS has no subscription of any kind - a
 * consumer is whoever calls ReceiveMessage, and the service keeps no record of
 * who that was - and AWS runs the service, so there is no node, no process and
 * no setting an operator here could read or change.
 */
func TestConnDeclaresNoConceptSQSDoesNotHave(t *testing.T) {
	absent := []struct {
		capability model.Capability
		because    string
	}{
		{
			model.CapSubscriptionList,
			"there are no subscriptions. A consumer is whoever calls " +
				"ReceiveMessage with a credential that allows it; nothing is " +
				"registered, named or remembered, so there is no set to list.",
		},
		{
			model.CapSubscriptionCreate,
			"and nothing to create: a reader starts reading, which is the whole " +
				"of what joining looks like here.",
		},
		{
			model.CapSubscriptionDelete,
			"nor to delete. A consumer that stops calling has left, and the queue " +
				"never knew it was there.",
		},
		{
			model.CapSubscriptionLag,
			"a queue's depth is the backlog and it belongs to the queue, not to a " +
				"reader of it. Every consumer of an SQS queue shares one backlog, " +
				"so there is no per-subscription lag to report.",
		},
		{
			model.CapSubscriptionRuntime,
			"same reason: with no subscription there is no connected member to ask " +
				"what it is working on.",
		},
		{
			model.CapOffsetReset,
			"a queue keeps no read position. A message is delivered and then " +
				"deleted by the consumer, so there is no cursor to move.",
		},
		{
			model.CapSubscriptionPosition,
			"and no log to name a place in - a queue is a set of messages waiting, " +
				"not an ordered history that outlives them.",
		},
		{
			model.CapQueueOffset,
			"with no partitions there is not even a per-partition position to write.",
		},
		{
			model.CapOffsetClone,
			"there is no position to copy from one reader onto another.",
		},
		{
			model.CapPartitions,
			"a queue is not split. SQS spreads a queue across its own servers and " +
				"reports no shard, no count and no range - which is also why a " +
				"standard queue's order is not guaranteed.",
		},
		{
			model.CapMessageByID,
			"nothing fetches one message by id. A message id is assigned on send " +
				"and echoed on receive, and there is no call that takes one: the " +
				"only way to reach a message is to be handed it by ReceiveMessage.",
		},
		{
			model.CapMessageTrack,
			"there is no trace. SQS reports how many times a message has been " +
				"received and nothing about who received it or what they did.",
		},
		{
			model.CapMessageLiveTail,
			"tailing is an incremental read of a durable log, and a queue holds no " +
				"log to keep a cursor in. Reading twice is not re-reading - it takes " +
				"whatever is visible now.",
		},
		{
			model.CapLiveStream,
			"nothing is pushed. Every message this app could show was pulled, and a " +
				"pull that showed it also hid it from a real consumer for the " +
				"visibility timeout - which is a browse with a caveat, not a stream.",
		},
		{
			model.CapDLQ,
			"a dead-letter queue here is an ordinary queue another queue's redrive " +
				"policy points at. It is not named after a consumer group, because " +
				"there are none - so it is found by walking the topology, which is " +
				"the dead-letter capability this driver does declare.",
		},
		{
			model.CapMessageResend,
			"and there is no per-group retry path to put a copy back on. SQS " +
				"redrives a whole dead-letter queue as a background task against the " +
				"queues it came from, which is not one message handed to one reader.",
		},
		{
			model.CapPendingEntries,
			"in-flight messages are counted, never enumerated. " +
				"ApproximateNumberOfMessagesNotVisible says how many are held and " +
				"there is no call that names them or says who is holding them.",
		},
		{
			model.CapPendingAdmin,
			"and with no list to read there is nothing to acknowledge or reassign.",
		},
		{
			model.CapDestinationMove,
			"nothing drains one queue into another on demand. A redrive task moves " +
				"a dead-letter queue back to the queues its messages came from, which " +
				"is a repair with a fixed destination rather than a move the caller " +
				"chooses.",
		},
		{
			model.CapStreamTrim,
			"a queue keeps no log to trim. Emptying one is PurgeQueue, which " +
				"discards everything rather than naming a bound to keep, and that is " +
				"the purge capability this driver does declare.",
		},
		{
			model.CapQueueRebalance,
			"placement is AWS's. There is nothing to spread and no node to spread it " +
				"across that this app can see.",
		},
		{
			model.CapClusterTopology,
			"there is no cluster. SQS is a regional service with no node, address or " +
				"process an operator here could be shown, and a topology board would " +
				"have exactly one invented row on it.",
		},
		{
			model.CapClusterMetrics,
			"and no node to attribute a figure to. What SQS reports is per queue, " +
				"which the queues board already shows; the rest lives in CloudWatch, " +
				"which is a different service with a different credential.",
		},
		{
			model.CapDirectory,
			"there is no discovery tier. A region resolves to an endpoint by name, " +
				"and nothing has to be asked where a queue lives.",
		},
		{
			model.CapNodeConfig,
			"a queue's attributes are its settings and are already on its own page. " +
				"There is no node underneath with settings of its own.",
		},
		{
			model.CapNodeMaintenance,
			"nothing here is maintained by its user. Retention is a queue attribute " +
				"AWS enforces on its own schedule.",
		},
		{
			model.CapLogDirs,
			"AWS reports no storage figure at all - not size, not free space, not a " +
				"percentage. A queue is billed by request, not by what it holds.",
		},
		{
			model.CapClusterHealth,
			"SQS answers no question about itself. Service health is the AWS Health " +
				"Dashboard, which is a different API this connection is not signed " +
				"for.",
		},
		{
			model.CapClusterCensus,
			"there is no account-wide total. Every figure SQS reports is one queue's, " +
				"and summing them would mean a request per queue and a number that was " +
				"never true at any single moment.",
		},
		{
			model.CapClientInspect,
			"nothing holds a connection. Every call is a signed HTTPS request that " +
				"stands alone, so there is no session to list and none to close.",
		},
		{
			model.CapClientClose,
			"and nothing to disconnect for the same reason.",
		},
		{
			model.CapAccessControl,
			"access is IAM's, not the queue's. A queue carries a resource policy, " +
				"but who may call what is decided by identities in a service this " +
				"connection is not signed for - and a page editing half of that would " +
				"claim to control access it cannot see.",
		},
		{
			model.CapAccessDirectory,
			"same reason: the directory of principals is IAM, one service further out.",
		},
		{
			model.CapAclUsers,
			"and SQS keeps no users of its own to attach rules to.",
		},
		{
			model.CapNamespaceList,
			"a queue name is flat and unique within an account and region. There is " +
				"no tenant, vhost or account inside SQS for one to live in.",
		},
		{
			model.CapTransactions,
			"a send is one message, or a batch of ten that succeed and fail " +
				"individually. Nothing spans queues, so there is no transaction with " +
				"an identity to list.",
		},
		{
			model.CapQuotaList,
			"the limits are the service's own and are the same for every caller. " +
				"They are not stored per identity, cannot be read back, and nothing " +
				"here could change one.",
		},
		{
			model.CapConnectionScope,
			"the queue prefix on this form filters a listing; it does not re-point " +
				"the connection. A name outside it is still perfectly reachable, which " +
				"is not what a scope means anywhere else in this app.",
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

	if descriptor.Kind != model.KindSQS {
		t.Errorf("kind = %q, want sqs", descriptor.Kind)
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

/*
 * The first family with no address, and the whole reason it is worth having.
 *
 * model.DriverDescriptor.RequiresEndpoints reads the form rather than a list of
 * kinds, so the absence of an endpoint field here is what lets a profile save
 * with its Endpoints empty. An endpoint row added to this form - even an
 * optional one, even one that only ever held the override - would be a field
 * asking for an address that does not exist, and the override already has a
 * row of its own that writes into an option.
 */
func TestDescriptorAsksForNoAddress(t *testing.T) {
	descriptor := New().Descriptor()

	for _, field := range descriptor.Form {
		if field.Target == model.TargetEndpoints {
			t.Errorf("form field %q asks for an address, and SQS has none to give", field.Key)
		}
	}
	if descriptor.RequiresEndpoints() {
		t.Error("the descriptor demands an address, so a profile could not save without one")
	}
	// Nothing listens on a port either: the SDK resolves an HTTPS endpoint
	// from the region. A default here would be drawn beside a field that does
	// not exist.
	if descriptor.DefaultPort != "" {
		t.Errorf("default port = %q, and SQS has no port to default to", descriptor.DefaultPort)
	}
}

/*
 * The credential names, pinned.
 *
 * accessKey and secretKey are reserved for RocketMQ's ACL: they skip
 * applyCredentials' generic loop, are written only through SetACL, and are
 * filled from global settings for any profile that named no mechanism. A
 * family reusing them would have its own credentials cleared on save and
 * RocketMQ's global pair stamped on at dial time, and nothing else would go
 * red - which is why this asserts the names rather than trusting the form.
 */
func TestCredentialFieldsAvoidTheReservedACLNames(t *testing.T) {
	if SecretAccessKeyID == model.SecretAccessKey || SecretSecretAccessKey == model.SecretSecretKey {
		t.Fatal("this driver stores credentials under the names reserved for RocketMQ's ACL")
	}

	wanted := map[string]bool{
		SecretAccessKeyID:     false,
		SecretSecretAccessKey: false,
		SecretSessionToken:    false,
	}
	for _, field := range New().Descriptor().Form {
		if field.Target != model.TargetSecret {
			continue
		}
		if _, known := wanted[field.Key]; !known {
			t.Errorf("form collects a credential named %q, which this driver never reads", field.Key)
			continue
		}
		wanted[field.Key] = true
	}
	for key, drawn := range wanted {
		if !drawn {
			t.Errorf("this driver reads the credential %q and the form never collects it", key)
		}
	}
}

/*
 * Browsing has to carry its caveat, because the read is the same call a
 * consumer makes.
 *
 * ReceiveMessage is the only read SQS has. What it returns is hidden from
 * everyone else for the visibility timeout and its receive count goes up,
 * which counts towards the redrive policy - so a message browsed often enough
 * is dead-lettered with nothing having failed. The driver hands every message
 * straight back and still declares this: handing back is not instantaneous,
 * and the receive count does not come back down.
 */
func TestBrowsingCarriesTheCaveatThatItHidesMessages(t *testing.T) {
	declared := (&Conn{closed: make(chan struct{})}).declare()

	if !declared.Has(model.CapMessageQuery) {
		t.Fatal("browsing is not declared at all")
	}
	caveat, warned := declared.Caveat(model.CapMessageQuery)
	if !warned {
		t.Fatal("browsing is offered with no caveat, and it is not a non-destructive read")
	}
	if caveat != receiveHides {
		t.Errorf("caveat = %q, want %q", caveat, receiveHides)
	}
	if _, degraded := declared.DegradedReason(model.CapMessageQuery); degraded {
		t.Error("browsing is both supported and degraded")
	}
}

/*
 * The caveats cross a language boundary as keys and are resolved by the
 * renderer, so a sentence here reaches the screen as a sentence in the wrong
 * language - and the renderer's own key test cannot see them, because it only
 * scans literal t("...") calls in the frontend source.
 */
func TestCaveatsAreTranslationKeys(t *testing.T) {
	for _, caveat := range []string{receiveHides} {
		if caveat == "" {
			t.Error("a caveat is empty")
			continue
		}
		if !isTranslationKey(caveat) {
			t.Errorf("%q is a sentence, not an i18n key", caveat)
		}
	}
}

func isTranslationKey(text string) bool {
	const prefix = "mq.sqs."
	return strings.HasPrefix(text, prefix) && !strings.Contains(text, " ")
}

// A profile with no region cannot sign anything, so it has to be refused
// where the message can still name the field rather than at the first call.
func TestOpenRefusesAProfileWithNoRegion(t *testing.T) {
	if _, err := configOf(model.ConnectionProfile{Kind: model.KindSQS}); err == nil {
		t.Fatal("accepted a profile naming no region")
	}
}

// Half a key pair signs nothing. Falling through to the machine's own
// credentials would connect as whoever it is rather than as the account the
// form names, which is a worse outcome than an error.
func TestOpenRefusesHalfACredential(t *testing.T) {
	profile := model.ConnectionProfile{
		Kind:    model.KindSQS,
		Options: map[string]string{OptionRegion: "eu-west-1"},
	}
	profile.SetSecret(SecretAccessKeyID, "AKIA-example")

	if _, err := configOf(profile); err == nil {
		t.Fatal("accepted an access key id with no secret access key")
	}

	profile.SetSecret(SecretAccessKeyID, "")
	profile.SetSecret(SecretSecretAccessKey, "s3cret")
	if _, err := configOf(profile); err == nil {
		t.Fatal("accepted a secret access key with no access key id")
	}
}

// A profile with neither key is not an unfinished form: it is the ordinary way
// to run on a machine that already holds an AWS identity.
func TestAProfileWithNoKeysUsesTheDefaultChain(t *testing.T) {
	config, err := configOf(model.ConnectionProfile{
		Kind:    model.KindSQS,
		Options: map[string]string{OptionRegion: "eu-west-1"},
	})
	if err != nil {
		t.Fatalf("configOf: %v", err)
	}
	if config.static {
		t.Error("a profile with no credentials claims to sign with its own pair")
	}
}

func TestQueueNameOfReadsTheLastSegment(t *testing.T) {
	tests := []struct {
		identifier string
		want       string
	}{
		{"https://sqs.eu-west-1.amazonaws.com/123456789012/orders", "orders"},
		{"https://sqs.eu-west-1.amazonaws.com/123456789012/orders/", "orders"},
		{"arn:aws:sqs:eu-west-1:123456789012:orders-dlq", "orders-dlq"},
		{"arn:aws:sqs:eu-west-1:123456789012:orders.fifo", "orders.fifo"},
		{"orders", "orders"},
		{"  ", ""},
	}
	for _, test := range tests {
		if got := queueNameOf(test.identifier); got != test.want {
			t.Errorf("queueNameOf(%q) = %q, want %q", test.identifier, got, test.want)
		}
	}
}

// The suffix is the service's own rule rather than a convention, so the name
// is a reliable answer and one that costs no request.
func TestIsFIFOReadsTheSuffix(t *testing.T) {
	if !isFIFO("orders.fifo") {
		t.Error("orders.fifo is not recognised as a FIFO queue")
	}
	if isFIFO("orders-fifo") {
		t.Error("orders-fifo is a standard queue whose name merely looks FIFO")
	}
}
