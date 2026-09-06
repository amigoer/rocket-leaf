package googlepubsub

import (
	"strings"
	"testing"

	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/model"
)

// offlineConn is a connection with the family's declared capabilities and no
// project behind it. Conformance is a question about the type, not about a
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
 * What Pub/Sub has no concept of, and why.
 *
 * Every entry is a capability another family in this app declares and this one
 * must not, and each reason is about Pub/Sub rather than about how far the
 * driver has got. Without this list the cheapest way to add a family is to
 * copy a neighbour's capability set, and the result is a sidebar full of pages
 * that open onto nothing.
 *
 * Three roots cover nearly all of it. A topic stores nothing and a
 * subscription's state is a set of acknowledgements rather than a position in
 * a log; Pub/Sub keeps no record of who is reading or publishing; and Google
 * runs the service, so there is no node, no process and no setting an operator
 * here could read or change.
 *
 * The backlog is deliberately not on this list. It is degraded rather than
 * absent, because the family does have the concept - the number just lives in
 * Cloud Monitoring - and TestSubscriptionLagIsDegradedRatherThanInvented is
 * where that is pinned.
 */
func TestConnDeclaresNoConceptPubSubDoesNotHave(t *testing.T) {
	absent := []struct {
		capability model.Capability
		because    string
	}{
		{
			model.CapDestinationPurge,
			"a topic holds nothing to empty. What has a backlog is each subscription, " +
				"and emptying one is a seek - a different gesture on a different object, " +
				"which the position capability already covers.",
		},
		{
			model.CapDestinationMove,
			"nothing drains one topic into another. A dead-letter policy moves what a " +
				"subscription gave up on, which is the service's own decision rather than " +
				"a move the caller chooses.",
		},
		{
			model.CapStreamTrim,
			"there is no log to trim. A topic's retention is a duration the service " +
				"enforces, not a bound a caller names.",
		},
		{
			model.CapPartitions,
			"a topic is not split. Pub/Sub spreads one across its own servers and reports " +
				"no shard, no count and no range.",
		},
		{
			model.CapQueueRebalance,
			"placement is Google's. There is nothing to spread and no node to spread it " +
				"across that this app can see.",
		},
		{
			model.CapReassign,
			"and no replicas either: nothing in the API says where a message is kept.",
		},
		{
			model.CapSubscriptionRuntime,
			"nothing registers as a consumer. A subscription is read by whoever calls " +
				"Pull or holds a streaming pull open, and the service reports neither - so " +
				"there is no connected member to ask what it is working on.",
		},
		{
			model.CapOffsetClone,
			"a subscription has no position to copy. Its state is a set of " +
				"acknowledgements, and the only way to move it onto another subscription " +
				"is to take a snapshot - an object with an expiry that makes the topic " +
				"hold everything it could restore - and seek to it, which is the two " +
				"visible steps the position control already offers.",
		},
		{
			model.CapQueueOffset,
			"with no partitions there is not even a per-partition position to write.",
		},
		{
			model.CapMessageTrack,
			"there is no trace. Pub/Sub reports how many times a message has been " +
				"delivered and nothing about who received it or what they did.",
		},
		{
			model.CapMessageLiveTail,
			"tailing is an incremental read of a durable log, and a subscription holds no " +
				"log to keep a cursor in. Reading twice is not re-reading - it takes " +
				"whatever is undelivered now.",
		},
		{
			model.CapLiveStream,
			"a streaming pull is not a push. It is the same delivery an ordinary pull is, " +
				"with the same consequence for a real consumer - which is the browse page's " +
				"caveat rather than a second kind of read.",
		},
		{
			model.CapDLQ,
			"a dead-letter topic here is an ordinary topic a subscription's policy points " +
				"at. Nothing is named after the subscription, so it is found by walking the " +
				"topology - which is the dead-letter capability this driver does declare.",
		},
		{
			model.CapMessageResend,
			"and there is no per-subscription retry path to put a copy back on. A dead " +
				"letter is an ordinary message on another topic; putting it back means " +
				"publishing it again, which the send console does.",
		},
		{
			model.CapMessageReplay,
			"there is nothing connected to hand a message to. The service knows of no " +
				"consumer, so there is no listener to run and no result to report.",
		},
		{
			model.CapPendingEntries,
			"outstanding deliveries are not enumerable. A message is held for its ack " +
				"deadline and there is no call that lists what is being held or by whom.",
		},
		{
			model.CapPendingAdmin,
			"and with no list to read there is nothing to acknowledge or reassign.",
		},
		{
			model.CapDelayedDelivery,
			"pub/sub cannot hold a message back. It is delivered as soon as there is a " +
				"subscription to deliver it to, and a console that took a delay would " +
				"report holding back a message that went out at once.",
		},
		{
			model.CapEntryPublish,
			"a message is a body and a table of string attributes, not an ordered list of " +
				"named fields with an id the caller chooses.",
		},
		{
			model.CapProducerInspect,
			"nothing records who is publishing. A publisher authenticates, sends, and is " +
				"forgotten.",
		},
		{
			model.CapClusterTopology,
			"there is no cluster. Pub/Sub is a global service with no node, address or " +
				"process an operator here could be shown, and a topology board would have " +
				"exactly one invented row on it.",
		},
		{
			model.CapClusterMetrics,
			"and no node to attribute a figure to. What the admin API reports is the shape " +
				"of the topology; every number about it is in Cloud Monitoring, which is a " +
				"different API with a different credential.",
		},
		{
			model.CapDirectory,
			"there is no discovery tier. Every project is reached at the same address, and " +
				"nothing has to be asked where a topic lives.",
		},
		{
			model.CapNodeConfig,
			"a topic's and a subscription's settings are already on their own pages. There " +
				"is no node underneath with settings of its own.",
		},
		{
			model.CapNodeMaintenance,
			"nothing here is maintained by its user. Retention is enforced on Google's own " +
				"schedule.",
		},
		{
			model.CapLogDirs,
			"Google reports no storage figure at all - not size, not free space, not a " +
				"percentage.",
		},
		{
			model.CapClusterHealth,
			"Pub/Sub answers no question about itself. Service health is the Google Cloud " +
				"status dashboard, which is not an API this connection is signed for.",
		},
		{
			model.CapClusterCensus,
			"there is no project-wide total. Every figure worth having is per topic or per " +
				"subscription, and summing them would mean a request each and a number that " +
				"was never true at any single moment.",
		},
		{
			model.CapClientInspect,
			"nothing holds a connection this app can see. A publisher and a subscriber each " +
				"authenticate and are forgotten, so there is no session to list.",
		},
		{
			model.CapClientClose,
			"and nothing to disconnect for the same reason.",
		},
		{
			model.CapAccessControl,
			"access is IAM's, not the topic's. A topic and a subscription each carry an IAM " +
				"policy, but who may call what is decided by principals in a service this " +
				"connection is not signed for - and a page editing half of that would claim " +
				"to control access it cannot see.",
		},
		{
			model.CapAccessDirectory,
			"same reason: the directory of principals is IAM, one service further out.",
		},
		{
			model.CapAclUsers,
			"and Pub/Sub keeps no users of its own to attach rules to.",
		},
		{
			model.CapNamespaceList,
			"the project is the boundary and one connection is one project. There is no " +
				"tenant, vhost or namespace inside Pub/Sub for a topic to live in.",
		},
		{
			model.CapPolicyList,
			"settings are set on the object rather than matched to it by pattern. There is " +
				"no policy to list.",
		},
		{
			model.CapRouting,
			"there is no exchange. A publish reaches every subscription on the topic; what " +
				"narrows that is a subscription's own filter, which is a field on it rather " +
				"than a routing topology.",
		},
		{
			model.CapDefinitionsExport,
			"nothing hands back the project's topology as one document, and nothing takes " +
				"one back.",
		},
		{
			model.CapReplication,
			"there is no shovel and no federation. Moving messages between projects means " +
				"running something that reads one and publishes to the other.",
		},
		{
			model.CapStreamClients,
			"there is no second protocol. Everything reads Pub/Sub over the same API, so " +
				"there is no set of clients the ordinary listing cannot see.",
		},
		{
			model.CapTransactions,
			"a publish is one message. Nothing spans topics, so there is no transaction " +
				"with an identity to list.",
		},
		{
			model.CapQuotaList,
			"the limits are the service's own and are set per project in a different " +
				"console. They are not stored on a topic, cannot be read back here, and " +
				"nothing in this API could change one.",
		},
		{
			model.CapConnectionScope,
			"the name prefix on this form filters a listing; it does not re-point the " +
				"connection. A name outside it is still perfectly reachable, which is not " +
				"what a scope means anywhere else in this app.",
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

	if descriptor.Kind != model.KindGooglePubSub {
		t.Errorf("kind = %q, want google-pubsub", descriptor.Kind)
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
 * The second family with no address, and the reason the rule is worth having
 * twice.
 *
 * model.DriverDescriptor.RequiresEndpoints reads the form rather than a list of
 * kinds, so the absence of an endpoint field here is what lets a profile save
 * with its Endpoints empty. An endpoint row added to this form - even an
 * optional one, even one that only ever held the emulator host - would be a
 * field asking for an address that does not exist, and the emulator host
 * already has a row of its own that writes into an option.
 */
func TestDescriptorAsksForNoAddress(t *testing.T) {
	descriptor := New().Descriptor()

	for _, field := range descriptor.Form {
		if field.Target == model.TargetEndpoints {
			t.Errorf("form field %q asks for an address, and Pub/Sub has none to give", field.Key)
		}
	}
	if descriptor.RequiresEndpoints() {
		t.Error("the descriptor demands an address, so a profile could not save without one")
	}
	// Nothing listens on a port either: the client resolves
	// pubsub.googleapis.com over HTTPS for every project there is. A default
	// here would be drawn beside a field that does not exist.
	if descriptor.DefaultPort != "" {
		t.Errorf("default port = %q, and Pub/Sub has no port to default to", descriptor.DefaultPort)
	}
}

/*
 * The credential name, pinned.
 *
 * accessKey and secretKey are reserved for RocketMQ's ACL: they skip
 * applyCredentials' generic loop, are written only through SetACL, and are
 * filled from global settings for any profile that named no mechanism. A
 * family reusing them would have its own credential cleared on save and
 * RocketMQ's global pair stamped on at dial time, and nothing else would go
 * red - which is why this asserts the name rather than trusting the form.
 */
func TestCredentialFieldAvoidsTheReservedACLNames(t *testing.T) {
	if SecretCredentialsJSON == model.SecretAccessKey || SecretCredentialsJSON == model.SecretSecretKey {
		t.Fatal("this driver stores its credential under a name reserved for RocketMQ's ACL")
	}

	drawn := 0
	for _, field := range New().Descriptor().Form {
		if field.Target != model.TargetSecret {
			continue
		}
		if field.Key != SecretCredentialsJSON {
			t.Errorf("form collects a credential named %q, which this driver never reads", field.Key)
			continue
		}
		drawn++
	}
	if drawn != 1 {
		t.Errorf("the form draws %d credential fields, want exactly the service account key", drawn)
	}
}

// A profile with no project cannot name a single resource, so it has to be
// refused where the message can still name the field rather than at the first
// call.
func TestOpenRefusesAProfileWithNoProject(t *testing.T) {
	if _, err := configOf(model.ConnectionProfile{Kind: model.KindGooglePubSub}); err == nil {
		t.Fatal("accepted a profile naming no project")
	}
}

/*
 * A service account key is a JSON document, and the form takes the document
 * rather than a path to it.
 *
 * Checked here because the client library reports a malformed key as a
 * credentials-source failure that names no field at all, which reads as "this
 * machine has no Google identity" - the opposite of what happened.
 */
func TestOpenRefusesACredentialThatIsNotJSON(t *testing.T) {
	profile := model.ConnectionProfile{
		Kind:    model.KindGooglePubSub,
		Options: map[string]string{OptionProjectID: "my-project"},
	}
	profile.SetSecret(SecretCredentialsJSON, "/home/me/key.json")

	if _, err := configOf(profile); err == nil {
		t.Fatal("accepted a path where the key itself belongs")
	}

	profile.SetSecret(SecretCredentialsJSON, `{"type":"service_account","project_id":"my-project"}`)
	config, err := configOf(profile)
	if err != nil {
		t.Fatalf("configOf: %v", err)
	}
	if len(config.credentials) == 0 {
		t.Error("a profile carrying a service account key reports none")
	}
}

// A profile with no key is not an unfinished form: it is the ordinary way to
// run on a machine that already holds a Google identity, which is what
// Application Default Credentials is for.
func TestAProfileWithNoKeyUsesApplicationDefaultCredentials(t *testing.T) {
	config, err := configOf(model.ConnectionProfile{
		Kind:    model.KindGooglePubSub,
		Options: map[string]string{OptionProjectID: "my-project"},
	})
	if err != nil {
		t.Fatalf("configOf: %v", err)
	}
	if len(config.credentials) != 0 {
		t.Error("a profile with no credential claims to carry one")
	}
}

func TestShortNameReadsTheLastSegment(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"projects/my-project/topics/orders", "orders"},
		{"projects/my-project/subscriptions/orders-worker", "orders-worker"},
		{"projects/my-project/snapshots/before-replay", "before-replay"},
		{"orders", "orders"},
		{"  ", ""},
	}
	for _, test := range tests {
		if got := shortName(test.path); got != test.want {
			t.Errorf("shortName(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

/*
 * The backlog is degraded, always, and it is worth saying why in a test.
 *
 * num_undelivered_messages is a Cloud Monitoring metric. It is not a field on
 * the Subscription this API returns and there is no call anywhere in Pub/Sub
 * that reports it, so the honest states are "degraded with a reason" and "not
 * offered" - and degraded is the truer of the two, because the family does
 * have the concept.
 *
 * What is not honest is a number. The only way to produce one here would be to
 * pull the backlog and count it, which would deliver every message counted and
 * hide it from the consumer that should have had it.
 */
func TestSubscriptionLagIsDegradedRatherThanInvented(t *testing.T) {
	declared := (&Conn{closed: make(chan struct{})}).declare()

	if declared.Has(model.CapSubscriptionLag) {
		t.Fatal("the backlog is offered as a figure, and the admin API reports none")
	}
	reason, degraded := declared.DegradedReason(model.CapSubscriptionLag)
	if !degraded {
		t.Fatal("the backlog is neither supported nor explained, so the page says nothing at all")
	}
	if reason != lagInMonitoring {
		t.Errorf("reason = %q, want %q", reason, lagInMonitoring)
	}
	// The subscriptions page still has to be reachable: listing, creating and
	// deleting all work, and only the one figure is missing.
	if !declared.Has(model.CapSubscriptionList) {
		t.Error("the subscriptions page is unreachable because one figure is missing")
	}
}

/*
 * Seek is two operations and both are declared, which took a correction.
 *
 * A timestamp names a moment; a snapshot names the place itself. The first
 * looked like something an emulator could not do at all - it answers
 * Unimplemented - and the hole turned out to be one subscription's setting
 * rather than the endpoint's: an ordered subscription cannot be sought to a
 * time there, and every other one can. So neither capability is narrowed, and
 * the refusal is an error at the call with a message naming message ordering.
 *
 * Declaring both is what the difference is for. CapOffsetReset describes a
 * moment and lets the service place it; CapSubscriptionPosition names a
 * snapshot the caller already holds, which is the only target the emulator
 * serves for an ordered subscription.
 */
func TestBothHalvesOfSeekAreDeclared(t *testing.T) {
	declared := (&Conn{closed: make(chan struct{})}).declare()

	if !declared.Has(model.CapOffsetReset) {
		t.Error("seeking to a moment is not offered, and Pub/Sub serves it")
	}
	if !declared.Has(model.CapSubscriptionPosition) {
		t.Error("seeking to a snapshot is not offered, and it is the only target that always works")
	}
	for _, capability := range []model.Capability{
		model.CapOffsetReset, model.CapSubscriptionPosition,
	} {
		if _, degraded := declared.DegradedReason(capability); degraded {
			t.Errorf("%s is both supported and degraded", capability)
		}
	}
}

/*
 * The reasons cross a language boundary as keys and are resolved by the
 * renderer, so a sentence here reaches the screen as a sentence in the wrong
 * language - and the renderer's own key test cannot see them, because it only
 * scans literal t("...") calls in the frontend source.
 */
func TestDegradedReasonsAreTranslationKeys(t *testing.T) {
	for _, reason := range []string{lagInMonitoring} {
		if reason == "" {
			t.Error("a reason is empty")
			continue
		}
		if !isTranslationKey(reason) {
			t.Errorf("%q is a sentence, not an i18n key", reason)
		}
	}
}

func isTranslationKey(text string) bool {
	const prefix = "mq.google-pubsub."
	return strings.HasPrefix(text, prefix) && !strings.Contains(text, " ")
}

/*
 * Browsing has to carry its caveat, because the read is the same call a
 * consumer makes.
 *
 * Pull is the only read Pub/Sub has. What it returns is held away from every
 * other reader for the subscription's ack deadline and its delivery attempt
 * goes up, which counts towards the dead-letter policy - so a message browsed
 * often enough is dead-lettered with nothing having failed. The driver hands
 * every message straight back and still declares this: handing back is not
 * instantaneous, and the delivery attempt does not come back down.
 */
func TestBrowsingCarriesTheCaveatThatItDelivers(t *testing.T) {
	declared := (&Conn{closed: make(chan struct{})}).declare()

	if !declared.Has(model.CapMessageQuery) {
		t.Fatal("browsing is not declared at all")
	}
	caveat, warned := declared.Caveat(model.CapMessageQuery)
	if !warned {
		t.Fatal("browsing is offered with no caveat, and it is not a non-destructive read")
	}
	if caveat != pullDelivers {
		t.Errorf("caveat = %q, want %q", caveat, pullDelivers)
	}
	if _, degraded := declared.DegradedReason(model.CapMessageQuery); degraded {
		t.Error("browsing is both supported and degraded")
	}
	if !isTranslationKey(caveat) {
		t.Errorf("%q is a sentence, not an i18n key", caveat)
	}
}

// Reading one message by id is not offered, and the reason is the service's:
// an id is assigned on publish and echoed on delivery, and no call takes one.
func TestMessageByIDIsNotOffered(t *testing.T) {
	if offlineConn().Capabilities().Has(model.CapMessageByID) {
		t.Error("declares a lookup by message id, and nothing in Pub/Sub indexes one")
	}
	if _, err := offlineConn().MessageByID(t.Context(), "orders", "1"); err == nil {
		t.Error("returned a message for an id nothing can look up")
	}
}

/*
 * The sidebar contract, from the Go side.
 *
 * The list below is the one frontend/src/mq/navigation.google-pubsub.test.ts
 * holds, and that test asserts which pages those capabilities make reachable.
 * This one asserts the driver still declares exactly them.
 *
 * Neither half is worth much alone. A capability dropped here takes a finished
 * page out of the sidebar and nothing else notices; a page added there with no
 * capability behind it is drawn and fails when opened. Together they cannot
 * drift without one of them going red.
 *
 * The failure messages say what to do rather than what is different, because
 * the fix is never in this file alone.
 */
func TestCapabilitiesMatchTheSidebarContract(t *testing.T) {
	sidebar := []string{
		"destination.list",
		"destination.create",
		"destination.update",
		"destination.delete",
		"subscription.list",
		"subscription.create",
		"subscription.delete",
		"subscription.position",
		"subscription.resetOffset",
		"message.query",
		"message.publish",
		"message.dlqTopology",
	}

	declared := make(map[string]bool, len(capabilities()))
	for _, capability := range capabilities() {
		declared[string(capability)] = true
	}
	expected := make(map[string]bool, len(sidebar))
	for _, capability := range sidebar {
		expected[capability] = true
	}

	for _, capability := range sidebar {
		if !declared[capability] {
			t.Errorf("the sidebar expects %s and the driver no longer declares it; "+
				"restore it or drop the page, and update navigation.google-pubsub.test.ts "+
				"in the same commit", capability)
		}
	}
	for capability := range declared {
		if !expected[capability] {
			t.Errorf("the driver declares %s and the sidebar contract does not list it; "+
				"add it to navigation.google-pubsub.test.ts in the same commit", capability)
		}
	}
}
