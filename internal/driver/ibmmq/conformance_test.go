package ibmmq

import (
	"strings"
	"testing"

	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/model"
)

// offlineConn is a connection with the family's declared capabilities and no
// queue manager behind it. Conformance is a question about the type, not about
// a server, so it must be answerable with nothing running.
func offlineConn() *Conn {
	conn := &Conn{qmgr: "QM1", closed: make(chan struct{})}
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

	if descriptor.Kind != model.KindIBMMQ {
		t.Errorf("kind = %q, want ibmmq", descriptor.Kind)
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
 * This family has an address, and it has to keep saying so.
 *
 * It would be easy to file IBM MQ with the hosted families: a connection names
 * a queue manager and a channel, neither of which is a hostname, and the
 * driver never opens 1414. But what it does open is https://host:9443 - a DNS
 * host and a TCP port a user types into the form - and every path is built on
 * it. model.DriverDescriptor.RequiresEndpoints reads the form rather than a
 * list of kinds, so a required endpoint field here is what makes the
 * connection service demand an address, and dropping it would let a profile
 * save with nothing to dial.
 */
func TestDescriptorAsksForAnAddress(t *testing.T) {
	descriptor := New().Descriptor()

	var endpoints *model.FormField
	for index, field := range descriptor.Form {
		if field.Target == model.TargetEndpoints {
			endpoints = &descriptor.Form[index]
		}
	}
	if endpoints == nil {
		t.Fatal("the form asks for no address, and the mqweb server is one this driver dials")
	}
	if !endpoints.Required {
		t.Error("the address is optional, and there is nothing this driver could derive it from")
	}
	if !descriptor.RequiresEndpoints() {
		t.Error("the descriptor does not demand an address, so a profile could save with none")
	}
	// The queue manager is not the address and must not be collected as one:
	// it is a path segment on the server the endpoint names.
	if endpoints.Key == OptionQueueManager {
		t.Error("the queue manager is being collected as the address")
	}
	if descriptor.DefaultPort != defaultPort {
		t.Errorf("default port = %q, want the mqweb server's %q", descriptor.DefaultPort, defaultPort)
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
	for _, key := range []string{
		SecretUsername, SecretPassword, SecretMessagingUsername, SecretMessagingPassword,
	} {
		if key == model.SecretAccessKey || key == model.SecretSecretKey {
			t.Fatalf("this driver stores %q, a name reserved for RocketMQ's ACL", key)
		}
	}

	wanted := map[string]bool{
		SecretUsername:          false,
		SecretPassword:          false,
		SecretMessagingUsername: false,
		SecretMessagingPassword: false,
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

// A profile with no address cannot dial anything, so it has to be refused
// where the message can still name the field rather than at the first call.
func TestOpenRefusesAProfileWithNoAddress(t *testing.T) {
	if _, err := configOf(model.ConnectionProfile{Kind: model.KindIBMMQ}); err == nil {
		t.Fatal("accepted a profile naming no mqweb address")
	}
}

/*
 * The messaging credential falls back to the administrative one.
 *
 * That is the common deployment - one account mapped to both mqweb roles - and
 * the fallback is what keeps the second pair optional on the form. What it must
 * not do is fall back when only half the pair was typed: a user who filled in a
 * messaging username and no password meant that user, and silently sending the
 * administrative one would authenticate as somebody else.
 */
func TestMessagingCredentialsFallBackToTheAdministrativeOnes(t *testing.T) {
	profile := model.ConnectionProfile{
		Kind:      model.KindIBMMQ,
		Endpoints: "https://mq.example:9443",
		Auth:      model.AuthConfig{Mechanism: model.AuthPlain},
	}
	profile.SetSecret(SecretUsername, "admin")
	profile.SetSecret(SecretPassword, "adminpw")

	config, err := configOf(profile)
	if err != nil {
		t.Fatalf("configOf: %v", err)
	}
	if config.messaging != config.admin {
		t.Errorf("messaging credential = %+v, want the administrative one", config.messaging)
	}

	profile.SetSecret(SecretMessagingUsername, "app")
	profile.SetSecret(SecretMessagingPassword, "apppw")
	config, err = configOf(profile)
	if err != nil {
		t.Fatalf("configOf: %v", err)
	}
	if config.messaging.username != "app" || config.messaging.password != "apppw" {
		t.Errorf("messaging credential = %+v, want the one the form collected", config.messaging)
	}
	if config.admin.username != "admin" {
		t.Errorf("administrative credential = %+v, want the one the form collected", config.admin)
	}
}

/*
 * An address typed without a scheme is https, not http.
 *
 * Every other family's endpoint field takes a host and port, so users type one
 * here too. Defaulting to http would send a Basic credential in clear to a
 * server that is TLS-only by default, and the failure would look like a
 * connection refused rather than like the mistake it is.
 */
func TestAnAddressWithNoSchemeIsHTTPS(t *testing.T) {
	tests := map[string]string{
		"127.0.0.1:9443":         "https://127.0.0.1:9443",
		"https://mq.example":     "https://mq.example",
		"http://mq.example:9080": "http://mq.example:9080",
		" mq.example:9443 ":      "https://mq.example:9443",
	}
	for given, want := range tests {
		if got := firstEndpoint(given); got != want {
			t.Errorf("firstEndpoint(%q) = %q, want %q", given, got, want)
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
 * The degraded reasons and caveats cross a language boundary as keys and are
 * resolved by the renderer, so a sentence here reaches the screen as a sentence
 * in the wrong language - and the renderer's own key test cannot see them,
 * because it only scans literal t("...") calls in the frontend source.
 */
func TestDegradedReasonsAndCaveatsAreTranslationKeys(t *testing.T) {
	for _, key := range []string{
		messagingForbidden, messagingRefused, browseCharacterOnly, sendQueueOnly,
	} {
		if !isTranslationKey(key) {
			t.Errorf("%q is a sentence, not an i18n key", key)
		}
	}
}

func isTranslationKey(text string) bool {
	const prefix = "mq.ibmmq."
	return strings.HasPrefix(text, prefix) && !strings.Contains(text, " ")
}

/*
 * The caveat browsing carries, and - just as importantly - the one it does not.
 *
 * Every other family here reached through a management API warns that looking
 * at a message takes it away from a consumer: SQS's ReceiveMessage hides what
 * it read and raises its receive count, Pub/Sub's Pull does the same and
 * counts towards being dead-lettered. Neither is true here. IBM MQ's messaging
 * interface has both operations and this driver uses the non-destructive one:
 * GET leaves the queue's depth alone, the messages stay in order, and any
 * number of readers can look at the same one.
 *
 * What is true is the other thing. The mqweb server carries character data and
 * nothing else, so a message the queue manager stored in any other format is
 * listed with its identifier and refused when opened - which is the ordinary
 * state of every dead letter on the queue manager.
 */
func TestBrowsingWarnsAboutTheFormatAndNotAboutConsuming(t *testing.T) {
	declared := (&Conn{qmgr: "QM1", closed: make(chan struct{})}).declare("")

	if !declared.Has(model.CapMessageQuery) {
		t.Fatal("browsing is not declared at all")
	}
	caveat, warned := declared.Caveat(model.CapMessageQuery)
	if !warned {
		t.Fatal("browsing is offered with no caveat, and the server will refuse some bodies")
	}
	if caveat != browseCharacterOnly {
		t.Errorf("caveat = %q, want %q", caveat, browseCharacterOnly)
	}
	// The keys of the families whose browse does take a message away, named so
	// a copied caveat cannot pass: an MQ browse takes nothing.
	for _, borrowed := range []string{
		"mq.sqs.caveat.receiveHides",
		"mq.google-pubsub.caveat.pullDelivers",
		"mq.rabbitmq.caveat.browseAltersQueue",
	} {
		if caveat == borrowed {
			t.Errorf("browsing carries %s, which says a read alters the queue; "+
				"an ibm mq browse leaves the depth alone", borrowed)
		}
	}
	if _, degraded := declared.DegradedReason(model.CapMessageQuery); degraded {
		t.Error("browsing is both supported and degraded")
	}
}

/*
 * The messaging tier is degraded rather than absent when the credential cannot
 * reach it, and everything else stays supported.
 *
 * That split is the whole point of the middle state here. mqweb authorises its
 * two interfaces separately, so a connection can administer every object on
 * the queue manager and be unable to read one message - and a page that simply
 * disappeared would send the reader looking for a bug in this app rather than
 * at a role mapping.
 */
func TestTheMessagingTierIsDegradedRatherThanDropped(t *testing.T) {
	conn := &Conn{qmgr: "QM1", closed: make(chan struct{})}
	declared := conn.declare(messagingForbidden)

	for _, capability := range messagingCapabilities() {
		if declared.Has(capability) {
			t.Errorf("%s is supported on a connection that cannot reach the messaging api", capability)
		}
		reason, degraded := declared.DegradedReason(capability)
		if !degraded {
			t.Errorf("%s is absent with no reason; the family has it and this endpoint does not", capability)
			continue
		}
		if reason != messagingForbidden {
			t.Errorf("%s is degraded as %q, want %q", capability, reason, messagingForbidden)
		}
	}

	// The administrative half is untouched: it is a different interface with a
	// different role, and it answered.
	for _, capability := range []model.Capability{
		model.CapDestinationList, model.CapDestinationCreate, model.CapChannels,
	} {
		if !declared.Has(capability) {
			t.Errorf("%s was dropped with the messaging tier, and it is not part of it", capability)
		}
	}

	// A caveat about browsing on a connection that cannot browse would be a
	// warning attached to a control that is not there.
	if _, warned := declared.Caveat(model.CapMessageQuery); warned {
		t.Error("browsing carries a caveat while it is degraded")
	}
}
