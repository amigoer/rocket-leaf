package kinesis

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

// The descriptor is read before anything is dialled, so it has to stand on its
// own: a form that writes into a target nothing reads, or a capability the
// connection cannot honour, would both surface as a dead control.
func TestDescriptorIsSelfConsistent(t *testing.T) {
	descriptor := New().Descriptor()

	if descriptor.Kind != model.KindKinesis {
		t.Errorf("kind = %q, want kinesis", descriptor.Kind)
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
 * The second family with no address, and it has to keep saying so.
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
			t.Errorf("form field %q asks for an address, and Kinesis has none to give", field.Key)
		}
	}
	if descriptor.RequiresEndpoints() {
		t.Error("the descriptor demands an address, so a profile could not save without one")
	}
	// Nothing listens on a port either: the SDK resolves an HTTPS endpoint
	// from the region. A default here would be drawn beside a field that does
	// not exist.
	if descriptor.DefaultPort != "" {
		t.Errorf("default port = %q, and Kinesis has no port to default to", descriptor.DefaultPort)
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

// A profile with no region cannot sign anything, so it has to be refused
// where the message can still name the field rather than at the first call.
func TestOpenRefusesAProfileWithNoRegion(t *testing.T) {
	if _, err := configOf(model.ConnectionProfile{Kind: model.KindKinesis}); err == nil {
		t.Fatal("accepted a profile naming no region")
	}
}

// Half a key pair signs nothing. Falling through to the machine's own
// credentials would connect as whoever it is rather than as the account the
// form names, which is a worse outcome than an error.
func TestOpenRefusesHalfACredential(t *testing.T) {
	profile := model.ConnectionProfile{
		Kind:    model.KindKinesis,
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
		Kind:    model.KindKinesis,
		Options: map[string]string{OptionRegion: "eu-west-1"},
	})
	if err != nil {
		t.Fatalf("configOf: %v", err)
	}
	if config.static {
		t.Error("a profile with no credentials claims to sign with its own pair")
	}
}

/*
 * A consumer's ARN is the stream's with its own name appended, so the last
 * segment is the wrong answer for half the ARNs this driver handles.
 */
func TestStreamNameOfReadsTheStreamSegment(t *testing.T) {
	tests := []struct {
		arn  string
		want string
	}{
		{"arn:aws:kinesis:eu-west-1:123456789012:stream/orders", "orders"},
		{"arn:aws:kinesis:eu-west-1:123456789012:stream/orders/consumer/analytics:1700000000", "orders"},
		{"orders", "orders"},
		{"  ", ""},
	}
	for _, test := range tests {
		if got := streamNameOf(test.arn); got != test.want {
			t.Errorf("streamNameOf(%q) = %q, want %q", test.arn, got, test.want)
		}
	}
}

// The form's label keys cross a language boundary, so a sentence here would
// reach the screen as a sentence in the wrong language.
func TestFormLabelsAreTranslationKeys(t *testing.T) {
	for _, field := range New().Descriptor().Form {
		if strings.Contains(field.LabelKey, " ") {
			t.Errorf("form field %q is labelled with a sentence, not an i18n key: %q",
				field.Key, field.LabelKey)
		}
	}
}

/*
 * The caveat browsing carries, and - just as importantly - the one it does not.
 *
 * Every other hosted family in this app warns that looking at a message takes
 * it away from a consumer: SQS's ReceiveMessage hides what it read and raises
 * its receive count, Pub/Sub's Pull does the same and counts towards being
 * dead-lettered. Neither is true here. GetRecords removes nothing, hides
 * nothing and marks nothing, and any number of readers can read the same
 * record until the retention period expires.
 *
 * What is true is the other thing, and it is why the capability is not left
 * bare: a shard allows five GetRecords a second and two megabytes a second,
 * shared with every classic consumer reading it, so a browse can throttle a
 * running application without having taken a single record from it.
 */
func TestBrowsingWarnsAboutTheReadQuotaAndNotAboutConsuming(t *testing.T) {
	declared := (&Conn{closed: make(chan struct{})}).declare()

	if !declared.Has(model.CapMessageQuery) {
		t.Fatal("browsing is not declared at all")
	}
	caveat, warned := declared.Caveat(model.CapMessageQuery)
	if !warned {
		t.Fatal("browsing is offered with no caveat, and it does spend a shard's read budget")
	}
	if caveat != readQuota {
		t.Errorf("caveat = %q, want %q", caveat, readQuota)
	}
	// The SQS and Pub/Sub keys, named so a copied caveat cannot pass: the
	// consequence they describe does not exist in this family.
	for _, borrowed := range []string{
		"mq.sqs.caveat.receiveHides",
		"mq.google-pubsub.caveat.pullDelivers",
	} {
		if caveat == borrowed {
			t.Errorf("browsing carries %s, which says a read takes the record away; "+
				"a kinesis read takes nothing", borrowed)
		}
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
	for _, caveat := range []string{readQuota} {
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
	const prefix = "mq.kinesis."
	return strings.HasPrefix(text, prefix) && !strings.Contains(text, " ")
}
