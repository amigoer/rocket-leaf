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
