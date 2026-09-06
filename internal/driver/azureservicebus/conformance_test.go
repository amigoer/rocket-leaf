package azureservicebus

import (
	"strings"
	"testing"

	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/model"
)

// offlineConn is a connection with the family's declared capabilities and no
// namespace behind it. Conformance is a question about the type, not about a
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

	if descriptor.Kind != model.KindAzureServiceBus {
		t.Errorf("kind = %q, want azure-servicebus", descriptor.Kind)
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
 * The third hosted family, and the first of them with an address.
 *
 * The two before it declare no endpoint field, and that is right for them: an
 * AWS region and a Google project are not addresses, and the form would have
 * been asking for something the user could not know. A Service Bus namespace
 * is one - myns.servicebus.windows.net resolves, and this driver opens an AMQP
 * connection to it and sends HTTPS requests to it. So the field exists, it is
 * required, and RequiresEndpoints reports it.
 *
 * This is asserted rather than assumed because the alternative was defensible
 * and would have been silently wrong: a namespace stashed in an option would
 * have left the connection list's address column blank on a connection that
 * has an address, and nothing would have gone red.
 */
func TestDescriptorAsksForAnAddress(t *testing.T) {
	descriptor := New().Descriptor()

	endpoints := 0
	for _, field := range descriptor.Form {
		if field.Target != model.TargetEndpoints {
			continue
		}
		endpoints++
		if !field.Required {
			t.Errorf("form field %q asks for the namespace but does not require it", field.Key)
		}
	}
	if endpoints != 1 {
		t.Errorf("the form draws %d endpoint fields, want exactly the namespace", endpoints)
	}
	if !descriptor.RequiresEndpoints() {
		t.Error("the descriptor lets a profile save with no namespace, and both clients dial one")
	}
	// No default port all the same. A namespace is named without one - the
	// SDK uses 5671 for AMQP and 443 for the management API - and the
	// emulator's two ports are a port on the endpoint and the option beside it.
	if descriptor.DefaultPort != "" {
		t.Errorf("default port = %q, and a namespace is named without a port", descriptor.DefaultPort)
	}
}

/*
 * The credential names, pinned.
 *
 * accessKey and secretKey are reserved for RocketMQ's ACL: they skip
 * applyCredentials' generic loop, are written only through SetACL, and are
 * filled from global settings for any profile that named no mechanism. A
 * family reusing them would have its own credential cleared on save and
 * RocketMQ's global pair stamped on at dial time, and nothing else would go
 * red - which is why this asserts the names rather than trusting the form.
 */
func TestCredentialFieldsAvoidTheReservedACLNames(t *testing.T) {
	for _, key := range []string{SecretSharedAccessKey, SecretConnectionString} {
		if key == model.SecretAccessKey || key == model.SecretSecretKey {
			t.Fatalf("this driver stores %q under a name reserved for RocketMQ's ACL", key)
		}
	}

	drawn := map[string]bool{}
	for _, field := range New().Descriptor().Form {
		if field.Target != model.TargetSecret {
			continue
		}
		switch field.Key {
		case SecretSharedAccessKey, SecretConnectionString:
			drawn[field.Key] = true
		default:
			t.Errorf("form collects a credential named %q, which this driver never reads", field.Key)
		}
	}
	if len(drawn) != 2 {
		t.Errorf("the form draws %d of the two credential fields", len(drawn))
	}
}

// A profile with no namespace names nothing to dial, so it has to be refused
// where the message can still name the field rather than at the first call.
func TestOpenRefusesAProfileWithNoNamespace(t *testing.T) {
	if _, err := configOf(model.ConnectionProfile{Kind: model.KindAzureServiceBus}); err == nil {
		t.Fatal("accepted a profile naming no namespace")
	}
}

/*
 * A profile with a namespace and no credential is an unfinished form, and this
 * is the family where that is true.
 *
 * SQS falls back to the AWS credential chain and Pub/Sub to Application
 * Default Credentials, so an empty key field on either is a real way to run.
 * Service Bus has no ambient credential of any kind: a shared access key or a
 * connection string is the whole of it, and a connection with neither would
 * fail its first call with a signature error naming nothing.
 */
func TestOpenRefusesAProfileWithNoCredential(t *testing.T) {
	_, err := configOf(model.ConnectionProfile{
		Kind:      model.KindAzureServiceBus,
		Endpoints: "my-namespace.servicebus.windows.net",
	})
	if err == nil {
		t.Fatal("accepted a profile carrying no credential at all")
	}
}

// The two ways to fill the form have to reach the same connection string, or
// one of them is a field that does nothing.
func TestBothWaysOfFillingTheFormReachOneConnectionString(t *testing.T) {
	parts := model.ConnectionProfile{
		Kind:      model.KindAzureServiceBus,
		Endpoints: "my-namespace.servicebus.windows.net",
		Options:   map[string]string{OptionSharedAccessKeyName: "Reader"},
	}
	parts.SetSecret(SecretSharedAccessKey, "c2VjcmV0")

	config, err := configOf(parts)
	if err != nil {
		t.Fatalf("configOf: %v", err)
	}
	for _, want := range []string{
		"Endpoint=sb://my-namespace.servicebus.windows.net",
		"SharedAccessKeyName=Reader",
		"SharedAccessKey=c2VjcmV0",
	} {
		if !strings.Contains(config.connectionString, want) {
			t.Errorf("composed string is missing %q: %s", want, config.connectionString)
		}
	}

	pasted := model.ConnectionProfile{
		Kind:      model.KindAzureServiceBus,
		Endpoints: "my-namespace.servicebus.windows.net",
	}
	pasted.SetSecret(SecretConnectionString,
		"Endpoint=sb://my-namespace.servicebus.windows.net/;SharedAccessKeyName=Reader;SharedAccessKey=c2VjcmV0")
	pastedConfig, err := configOf(pasted)
	if err != nil {
		t.Fatalf("configOf: %v", err)
	}
	if !strings.Contains(pastedConfig.connectionString, "SharedAccessKey=c2VjcmV0") {
		t.Errorf("a pasted string lost its key: %s", pastedConfig.connectionString)
	}
	// The pasted string wins outright rather than being merged with the
	// fields beside it: it carries an endpoint of its own, and a merge would
	// have to decide which of two disagreeing namespaces was meant.
	if pastedConfig.namespace != "my-namespace.servicebus.windows.net" {
		t.Errorf("namespace = %q", pastedConfig.namespace)
	}
}

// Something with no Endpoint= in it is not a connection string, and the SDK's
// own refusal names a key rather than the field the form draws.
func TestOpenRefusesSomethingThatIsNotAConnectionString(t *testing.T) {
	profile := model.ConnectionProfile{
		Kind:      model.KindAzureServiceBus,
		Endpoints: "my-namespace.servicebus.windows.net",
	}
	profile.SetSecret(SecretConnectionString, "my-namespace.servicebus.windows.net")

	if _, err := configOf(profile); err == nil {
		t.Fatal("accepted a bare hostname where the connection string belongs")
	}
}

/*
 * The emulator flag, which is not a testing convenience.
 *
 * The SDK refuses a plaintext AMQP port unless the connection string says
 * UseDevelopmentEmulator, so a connection that named an emulator management
 * host and did not get the flag would fail at the first send with a TLS
 * handshake error against a port serving no TLS.
 */
func TestNamingAnEmulatorTurnsOnTheDevelopmentFlag(t *testing.T) {
	profile := model.ConnectionProfile{
		Kind:      model.KindAzureServiceBus,
		Endpoints: "localhost:5672",
		Options:   map[string]string{OptionEmulatorManagement: "127.0.0.1:5300"},
	}
	profile.SetSecret(SecretSharedAccessKey, "SAS_KEY_VALUE")

	config, err := configOf(profile)
	if err != nil {
		t.Fatalf("configOf: %v", err)
	}
	if !config.emulator() {
		t.Fatal("a profile naming an emulator management host is not in emulator mode")
	}
	if !strings.Contains(config.connectionString, "UseDevelopmentEmulator=true") {
		t.Errorf("the emulator's connection string demands TLS: %s", config.connectionString)
	}

	plain := model.ConnectionProfile{
		Kind:      model.KindAzureServiceBus,
		Endpoints: "my-namespace.servicebus.windows.net",
	}
	plain.SetSecret(SecretSharedAccessKey, "c2VjcmV0")
	realConfig, err := configOf(plain)
	if err != nil {
		t.Fatalf("configOf: %v", err)
	}
	if realConfig.emulator() {
		t.Error("a profile naming no emulator is in emulator mode")
	}
	if strings.Contains(realConfig.connectionString, "UseDevelopmentEmulator") {
		t.Error("a real namespace was told to skip TLS")
	}
}

// The endpoint field takes what a user actually pastes into it, which is one
// of three spellings of the same namespace.
func TestNamespaceOfReadsWhateverWasPasted(t *testing.T) {
	tests := []struct {
		endpoints string
		want      string
	}{
		{"my-namespace.servicebus.windows.net", "my-namespace.servicebus.windows.net"},
		{"sb://my-namespace.servicebus.windows.net/", "my-namespace.servicebus.windows.net"},
		{"  my-namespace.servicebus.windows.net  ", "my-namespace.servicebus.windows.net"},
		{"localhost:5672", "localhost:5672"},
		// A second entry would be a second namespace, which is a second
		// connection; the field is a list because the type is shared.
		{"first.servicebus.windows.net,second.servicebus.windows.net", "first.servicebus.windows.net"},
		{"", ""},
	}
	for _, test := range tests {
		if got := namespaceOf(test.endpoints); got != test.want {
			t.Errorf("namespaceOf(%q) = %q, want %q", test.endpoints, got, test.want)
		}
	}
}

/*
 * A name field addresses an entity, never one of its sub-entities.
 *
 * $DeadLetterQueue and $Transfer/$DeadLetterQueue are reached by suffixing an
 * entity path, so a name typed with one in it would read a different entity
 * from the one the page named - and the dead letters have a page of their own
 * that addresses them deliberately.
 */
func TestRequiredNameRefusesASubEntityPath(t *testing.T) {
	if _, err := requiredName("queue", "orders/$DeadLetterQueue"); err == nil {
		t.Error("accepted a dead-letter path where an entity name belongs")
	}
	if _, err := requiredName("queue", "  "); err == nil {
		t.Error("accepted an empty name")
	}
	// A subscription's own path does contain slashes, so a slash alone is not
	// what is refused.
	name, err := requiredName("subscription", " orders/Subscriptions/worker ")
	if err != nil {
		t.Fatalf("requiredName: %v", err)
	}
	if name != "orders/Subscriptions/worker" {
		t.Errorf("name = %q", name)
	}
}

/*
 * The reasons cross a language boundary as keys and are resolved by the
 * renderer, so a sentence here reaches the screen as a sentence in the wrong
 * language - and the renderer's own key test cannot see them, because it only
 * scans literal t("...") calls in the frontend source.
 */
func TestDegradedReasonsAreTranslationKeys(t *testing.T) {
	for _, reason := range []string{countsNotInEmulator} {
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
	const prefix = "mq.azure-servicebus."
	return strings.HasPrefix(text, prefix) && !strings.Contains(text, " ")
}

/*
 * Browsing carries no caveat, and pinning that absence is the point of this
 * test.
 *
 * It is the one thing this family does that neither hosted family before it
 * could. SQS's only read is ReceiveMessage, which hides what it read for a
 * visibility timeout and raises its receive count; Pub/Sub's only read is
 * Pull, which holds what it read away from consumers and raises its delivery
 * attempt, counting towards being dead-lettered. Both had to warn that
 * browsing perturbs delivery.
 *
 * PeekMessages takes no lock, moves nothing, changes no delivery count, and a
 * consumer running at the same moment misses nothing. So there is no caveat -
 * and this asserts the absence rather than leaving it to be noticed, because
 * the way it would be lost is silent: swapping the peek for a receive would
 * still return messages, and the page would go on saying nothing while
 * browsing quietly started taking them.
 */
func TestBrowsingCarriesNoCaveatAtAll(t *testing.T) {
	for _, endpoint := range []*Conn{
		{closed: make(chan struct{}), config: clientConfig{namespace: "real.servicebus.windows.net"}},
		{closed: make(chan struct{}), config: clientConfig{
			namespace: "localhost:5672", emulatorManagement: "127.0.0.1:5300"}},
	} {
		declared := endpoint.declare()
		if !declared.Has(model.CapMessageQuery) {
			t.Fatal("browsing is not declared at all")
		}
		if caveat, warned := declared.Caveat(model.CapMessageQuery); warned {
			t.Errorf("browsing warns %q; a peek takes nothing, and a caveat here would be "+
				"either a lie or a sign the read stopped being a peek", caveat)
		}
		if _, degraded := declared.DegradedReason(model.CapMessageQuery); degraded {
			t.Error("browsing is both supported and degraded")
		}
	}

	// And no caveat anywhere else either: this family has none, so a caveat
	// appearing is a change worth reviewing rather than one to discover.
	live := (&Conn{closed: make(chan struct{})}).declare()
	if len(live.Caveats) != 0 {
		t.Errorf("the connection declares caveats %v, and this family has none", live.Caveats)
	}
}

// Reading one message by id is not offered, and the reason is the service's:
// a message id is whatever the sender put there, it need not be unique, and no
// call takes one. What addresses a message here is its sequence number.
func TestMessageByIDIsNotOffered(t *testing.T) {
	if offlineConn().Capabilities().Has(model.CapMessageByID) {
		t.Error("declares a lookup by message id, and Service Bus indexes none")
	}
	if _, err := offlineConn().MessageByID(t.Context(), "orders", "1"); err == nil {
		t.Error("returned a message for an id nothing can look up")
	}
}

// The browse resumes from a sequence number, so a malformed one has to be
// refused where the message can name the field rather than at the call.
func TestBrowseRefusesAStartingPositionThatIsNotASequence(t *testing.T) {
	_, err := offlineConn().QueryMessages(t.Context(), model.MessageQueryParams{
		Topic:   "orders",
		Filters: map[string]string{FilterFromSequence: "the beginning"},
	})
	if err == nil {
		t.Fatal("accepted a starting position that is not a sequence number")
	}
}

// What was browsed is what the rows say they came from, because a queue, a
// subscription and a dead-letter sub-entity are three different places and the
// messages page shows one at a time.
func TestBrowseLabelNamesWhatWasRead(t *testing.T) {
	tests := []struct {
		entity       string
		subscription string
		deadLetters  bool
		want         string
	}{
		{"orders", "", false, "orders"},
		{"orders", "", true, "orders/$DeadLetterQueue"},
		{"events", "worker", false, "events/worker"},
		{"events", "worker", true, "events/worker/$DeadLetterQueue"},
	}
	for _, test := range tests {
		got := browseLabel(test.entity, test.subscription, test.deadLetters)
		if got != test.want {
			t.Errorf("browseLabel(%q, %q, %v) = %q, want %q",
				test.entity, test.subscription, test.deadLetters, got, test.want)
		}
	}
}
