package azureservicebus

import (
	"errors"
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

/*
 * Which dead-letter shape this family has, and why it is the other one.
 *
 * CapDLQ is a per-entity store the broker names and fills;
 * CapDeadLetterTopology is an ordinary object that something else points at,
 * found by walking every object's configuration backwards. The two hosted
 * families before this one are both the second: an SQS dead-letter queue is a
 * queue another queue's redrive policy names, and a Pub/Sub one is a topic a
 * subscription's policy names. Either can be deleted, renamed or shared, and
 * an account that has configured none has an empty page.
 *
 * A Service Bus $DeadLetterQueue is none of that. Every queue and every
 * subscription is created with one, it is addressed by suffixing the entity's
 * own path, and it cannot be listed, sent to, renamed or shared - it goes when
 * the entity goes. Nothing points at it and there is nothing to invert.
 *
 * ForwardDeadLetteredMessagesTo is the one thing that looks like the other
 * shape, and it is not: it is an optional forwarding rule laid over the
 * built-in store, and an entity that sets it still has a $DeadLetterQueue.
 */
func TestDeadLettersAreAStoreRatherThanATopology(t *testing.T) {
	declared := offlineConn().Capabilities()

	if !declared.Has(model.CapDLQ) {
		t.Error("the per-entity dead-letter store is not declared, and every entity has one")
	}
	if declared.Has(model.CapDeadLetterTopology) {
		t.Error("declares the topology shape; nothing here points at a dead-letter queue, " +
			"because the broker names one for every entity")
	}
	if _, degraded := declared.DegradedReason(model.CapDeadLetterTopology); degraded {
		t.Error("degrades the topology shape, which implies the family has it")
	}
}

/*
 * There is no retry store, and the driver says so rather than showing one.
 *
 * RocketMQ moves a failed message to a %RETRY% topic per consumer group, which
 * is what DeadLetterReader's second method is for. Service Bus redelivers in
 * place: an abandoned message goes back into the same entity with its delivery
 * count raised, and only when that count passes the limit does it move
 * anywhere - into the dead-letter store. Answering this with the ordinary
 * backlog would show every waiting message under a name that says it failed.
 */
func TestThereIsNoRetryStoreToRead(t *testing.T) {
	_, err := offlineConn().RetryMessages(t.Context(), "orders", 10)
	if err == nil {
		t.Fatal("returned a retry backlog, and Service Bus keeps none")
	}
	if !errors.Is(err, errNoRetryStore) {
		t.Errorf("the refusal is not the one that explains why: %v", err)
	}
}

// A dead letter is addressed by entity path and sequence number, so both have
// to be refused where the message can name them rather than at the call.
func TestEntityPathSplitsAQueueFromASubscription(t *testing.T) {
	tests := []struct {
		path         string
		entity       string
		subscription string
		refused      bool
	}{
		{"orders", "orders", "", false},
		{"events/worker", "events", "worker", false},
		{"  events/worker  ", "events", "worker", false},
		{"", "", "", true},
		// Already a sub-entity path: appending $DeadLetterQueue to it would
		// address something that does not exist.
		{"orders/$DeadLetterQueue", "", "", true},
	}
	for _, test := range tests {
		entity, subscription, err := entityPath(test.path)
		if test.refused {
			if err == nil {
				t.Errorf("entityPath(%q) was accepted", test.path)
			}
			continue
		}
		if err != nil {
			t.Errorf("entityPath(%q): %v", test.path, err)
			continue
		}
		if entity != test.entity || subscription != test.subscription {
			t.Errorf("entityPath(%q) = (%q, %q), want (%q, %q)",
				test.path, entity, subscription, test.entity, test.subscription)
		}
	}
}

// The resend takes a sequence number, which is the only thing that addresses a
// message here - a message id is the sender's own and need not be unique.
func TestResendRefusesSomethingThatIsNotASequenceNumber(t *testing.T) {
	if _, err := offlineConn().ResendMessage(t.Context(), "orders", "", "", "abc"); err == nil {
		t.Error("accepted a message id where a sequence number belongs")
	}
}

/*
 * Rules are the routing topology, and the routing page is where they go.
 *
 * The two hosted families before this one declare no routing capability and
 * are right not to: a Pub/Sub subscription's filter is a string field set once
 * at creation, and an SQS queue has none at all. A Service Bus rule is an
 * object - it has a name, several may sit on one subscription, and each is a
 * filter plus an optional action that rewrites the message - so which messages
 * reach which subscription is a topology, and it maps onto the same page
 * RabbitMQ's exchanges and bindings do.
 */
func TestRulesAreDeclaredAsRouting(t *testing.T) {
	declared := offlineConn().Capabilities()

	for _, capability := range []model.Capability{model.CapRouting, model.CapRoutingAdmin} {
		if !declared.Has(capability) {
			t.Errorf("%s is not declared, and a rule is a routing decision made by an object",
				capability)
		}
	}
}

// A rule is deleted by name and $Default is the one the service creates, so it
// has to be nameable - unlike an entity, where a leading $ addresses a
// sub-entity and is refused.
func TestTheDefaultRuleCanBeNamed(t *testing.T) {
	name, err := requiredRuleName(" " + DefaultRuleName + " ")
	if err != nil {
		t.Fatalf("requiredRuleName(%s): %v", DefaultRuleName, err)
	}
	if name != DefaultRuleName {
		t.Errorf("name = %q", name)
	}
	// Any other sub-entity name is still refused: only this one is a rule.
	if _, err := requiredRuleName("$DeadLetterQueue"); err == nil {
		t.Error("accepted a sub-entity name as a rule")
	}
	// And an entity may never carry one.
	if _, err := requiredName("topic", DefaultRuleName); err == nil {
		t.Error("accepted $Default as an entity name")
	}
}

/*
 * A correlation filter is rendered the way its SQL equivalent would read, so
 * one column can show both kinds.
 *
 * Worth pinning because the empty case is not empty: a correlation filter that
 * sets no field matches everything, and printing nothing there would read as
 * "matches nothing" - the opposite.
 */
func TestCorrelationFiltersReadLikeTheirSQLEquivalent(t *testing.T) {
	if got := renderCorrelation(map[string]string{}); got != "1=1" {
		t.Errorf("an empty correlation filter renders as %q, and it matches everything", got)
	}
	got := renderCorrelation(map[string]string{"subject": "order", "colour": "red"})
	if got != "colour = 'red' AND subject = 'order'" {
		t.Errorf("rendered %q", got)
	}
}

/*
 * A topic has no exchange type, and the port has a field for one.
 *
 * Refused rather than ignored: a RabbitMQ exchange's type is the whole of how
 * it routes, and accepting one would let a form report that a fanout topic had
 * been created when what exists is a topic whose routing is entirely in the
 * rules on its subscriptions.
 */
func TestDeclaringAnExchangeRefusesWhatATopicDoesNotHave(t *testing.T) {
	conn := offlineConn()
	for _, spec := range []model.ExchangeSpec{
		{Name: "events", Type: "fanout"},
		{Name: "events", Transient: true},
		{Name: "events", AutoDelete: true},
	} {
		if err := conn.DeclareExchange(t.Context(), spec); err == nil {
			t.Errorf("accepted %#v, and a Service Bus topic has none of it", spec)
		}
	}
}

// The filter kind decides which text is sent, so a rule that names a kind and
// leaves the text out has to be refused rather than sent empty.
func TestARuleNeedsTheTextItsKindTakes(t *testing.T) {
	if _, err := ruleOptions("red", model.Binding{
		Arguments: map[string]string{ArgFilterType: FilterSQL},
	}); err == nil {
		t.Error("accepted a SQL rule with no expression")
	}
	if _, err := ruleOptions("orders", model.Binding{
		Arguments: map[string]string{ArgFilterType: FilterCorrelation},
	}); err == nil {
		t.Error("accepted a correlation rule matching nothing at all")
	}
	// No kind at all is the one that matches everything, which is what the
	// service's own $Default is - so it needs no text.
	if _, err := ruleOptions(DefaultRuleName, model.Binding{}); err != nil {
		t.Errorf("a rule with no kind was refused: %v", err)
	}
}

/*
 * The sidebar contract, from the Go side.
 *
 * The list below is the one frontend/src/mq/navigation.azure-servicebus.test.ts
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
		"subscription.lag",
		"message.query",
		"message.publish",
		"message.delayedDelivery",
		"message.dlq",
		"message.resend",
		"routing.exchanges",
		"routing.admin",
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
				"restore it or drop the page, and update navigation.azure-servicebus.test.ts "+
				"in the same commit", capability)
		}
	}
	for capability := range declared {
		if !expected[capability] {
			t.Errorf("the driver declares %s and the sidebar contract does not list it; "+
				"add it to navigation.azure-servicebus.test.ts in the same commit", capability)
		}
	}
}

/*
 * What Service Bus has no concept of, and why.
 *
 * Every entry is a capability another family in this app declares and this one
 * must not, and each reason is about Service Bus rather than about how far the
 * driver has got. Without this list the cheapest way to add a family is to
 * copy a neighbour's capability set, and the result is a sidebar full of pages
 * that open onto nothing.
 *
 * Three roots cover nearly all of it. Microsoft runs the service, so there is
 * no node, no process and no setting an operator here could read or change;
 * nothing registers as a reader, so no page can say who is consuming what; and
 * a message's place in an entity is a sequence number the service assigns
 * rather than a position anything can be moved to.
 *
 * The backlog is deliberately not on this list. It is declared, and degraded
 * only against an emulator, because a real namespace reports it -
 * TestBacklogNarrowsOnlyForAnEmulator is where that is pinned.
 */
func TestConnDeclaresNoConceptServiceBusDoesNotHave(t *testing.T) {
	absent := []struct {
		capability model.Capability
		because    string
	}{
		{
			model.CapDestinationPurge,
			"there is no purge call anywhere in the service. Emptying a queue means " +
				"receiving its messages, which is what the portal's own purge does and is " +
				"not something a button here should do quietly.",
		},
		{
			model.CapDestinationMove,
			"and nothing drains one entity into another on demand. Forwarding does move " +
				"messages, but it is a setting on the entity that applies to everything " +
				"arriving from then on rather than an operation on what is already there.",
		},
		{
			model.CapStreamTrim,
			"there is no log to trim. A message leaves an entity when it is completed, " +
				"expires, or is dead-lettered, and none of those is a bound a caller names.",
		},
		{
			model.CapPartitions,
			"partitioning is a flag rather than a count. The service spreads a partitioned " +
				"entity across its own brokers and reports no shard, no number and no range.",
		},
		{
			model.CapQueueRebalance,
			"placement is Microsoft's. There is nothing to spread and no node to spread it " +
				"across that this app can see.",
		},
		{
			model.CapReassign,
			"and no replicas either: nothing in the API says where a message is kept.",
		},
		{
			model.CapSubscriptionRuntime,
			"nothing registers as a consumer. A subscription is read by whoever opens a " +
				"receiver on it, and the service reports neither who nor how many - so " +
				"there is no connected member to ask what it is working on.",
		},
		{
			model.CapOffsetReset,
			"a subscription has no position to move. Its state is which messages have been " +
				"completed, and there is no call that takes it back to a moment in time.",
		},
		{
			model.CapSubscriptionPosition,
			"and none that takes it to a place either. A sequence number addresses a " +
				"message for a peek and cannot be seeked to.",
		},
		{
			model.CapOffsetClone,
			"with no position there is nothing to copy onto a second subscription.",
		},
		{
			model.CapQueueOffset,
			"and with no partitions, not even a per-partition position to write.",
		},
		{
			model.CapMessageByID,
			"a message id is the sender's own field. Nothing assigns one, nothing indexes " +
				"one, and it need not be unique - what addresses a message here is the " +
				"sequence number the browse resumes from.",
		},
		{
			model.CapMessageTrack,
			"there is no trace. The service reports how many times a message has been " +
				"delivered and nothing about who received it or what they did.",
		},
		{
			model.CapMessageLiveTail,
			"tailing is an incremental read of a durable log, and an entity is not one: a " +
				"completed message is gone, so reading twice from the same place does not " +
				"return the same thing twice.",
		},
		{
			model.CapLiveStream,
			"nothing is pushed. A receiver pulls, and what it pulls it takes - which is " +
				"the opposite of what a live view is for.",
		},
		{
			model.CapDeadLetterTopology,
			"a dead-letter store is part of its entity rather than an object something " +
				"points at. There is nothing to walk backwards through, which is why this " +
				"driver declares the per-entity shape instead.",
		},
		{
			model.CapMessageReplay,
			"there is nothing connected to hand a message to. The service knows of no " +
				"consumer, so there is no listener to run and no result to report.",
		},
		{
			model.CapPendingEntries,
			"outstanding deliveries are not enumerable. A message is locked for its lock " +
				"duration and there is no call that lists what is locked or by whom.",
		},
		{
			model.CapPendingAdmin,
			"and with no list to read there is nothing to acknowledge or reassign.",
		},
		{
			model.CapEntryPublish,
			"a message is a body with system fields and a table of properties, not an " +
				"ordered list of named fields with an id the caller chooses.",
		},
		{
			model.CapProducerInspect,
			"nothing records who is sending. A sender authenticates, sends, and is " +
				"forgotten.",
		},
		{
			model.CapClusterTopology,
			"there is no cluster. Service Bus is a managed service with no node, address " +
				"or process an operator here could be shown, and a topology board would " +
				"have exactly one invented row on it.",
		},
		{
			model.CapClusterMetrics,
			"and no node to attribute a figure to. Every number about throughput is in " +
				"Azure Monitor, which is a different API with a different credential.",
		},
		{
			model.CapDirectory,
			"there is no discovery tier. A namespace is one address and nothing has to be " +
				"asked where an entity lives.",
		},
		{
			model.CapNodeConfig,
			"an entity's settings are already on its own page. There is no node underneath " +
				"with settings of its own.",
		},
		{
			model.CapNodeMaintenance,
			"nothing here is maintained by its user. Expiry and auto-delete run on " +
				"Microsoft's schedule.",
		},
		{
			model.CapLogDirs,
			"the only storage figure is an entity's maximum size, which is a quota rather " +
				"than a disk. There is no free space and no percentage anywhere.",
		},
		{
			model.CapClusterHealth,
			"Service Bus answers no question about itself. Service health is the Azure " +
				"status page, which is not an API this connection is signed for.",
		},
		{
			model.CapClusterCensus,
			"there is no namespace-wide total. Every figure worth having is per entity, " +
				"and summing them would mean a request each and a number that was never " +
				"true at any single moment.",
		},
		{
			model.CapClientInspect,
			"nothing holds a connection this app can see. A sender and a receiver each " +
				"authenticate and are forgotten, so there is no session to list.",
		},
		{
			model.CapClientClose,
			"and nothing to disconnect for the same reason.",
		},
		{
			model.CapAccessControl,
			"access is a shared access policy on the namespace, which is what this " +
				"connection authenticated with. The management API will not enumerate them, " +
				"and a page editing half of that would claim to control access it cannot see.",
		},
		{
			model.CapAccessDirectory,
			"same reason, one layer further out: identity-based access is Entra ID's.",
		},
		{
			model.CapAclUsers,
			"and Service Bus keeps no users of its own to attach rules to.",
		},
		{
			model.CapNamespaceList,
			"the namespace is the boundary and one connection is one namespace. There is " +
				"no tenant or vhost inside it for an entity to live in.",
		},
		{
			model.CapPolicyList,
			"settings are set on the entity rather than matched to it by pattern. There is " +
				"no policy object to list.",
		},
		{
			model.CapDefinitionsExport,
			"nothing hands back a namespace's topology as one document, and nothing takes " +
				"one back. The emulator reads a config file at startup and the real service " +
				"has no counterpart to it.",
		},
		{
			model.CapReplication,
			"there is no shovel and no federation. Forwarding moves messages between " +
				"entities in one namespace and reaches no further.",
		},
		{
			model.CapStreamClients,
			"there is no second protocol. Everything speaks AMQP over the same namespace, " +
				"so there is no set of clients the ordinary listing cannot see.",
		},
		{
			model.CapTransactions,
			"a transaction here spans one entity and lives inside one AMQP session. " +
				"Nothing gives it an identity the service would list.",
		},
		{
			model.CapQuotaList,
			"the limits are the tier's own and are set per namespace in a different " +
				"console. They are not stored on an entity and nothing in this API could " +
				"change one.",
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

/*
 * The backlog is the one thing that varies by endpoint, and it varies the
 * right way round.
 *
 * Service Bus reports a subscription's active message count, so a real
 * namespace supports it. The emulator sends a CountDetails element whose five
 * children are renamed to tokens the SDK cannot read, so a connection to one
 * degrades it with a reason. Declaring it absent everywhere would be a lie
 * about Azure; declaring it supported everywhere would put a zero on a board
 * where there is no number at all.
 */
func TestBacklogNarrowsOnlyForAnEmulator(t *testing.T) {
	real := (&Conn{
		closed: make(chan struct{}),
		config: clientConfig{namespace: "real.servicebus.windows.net"},
	}).declare()
	if !real.Has(model.CapSubscriptionLag) {
		t.Error("a real namespace does not offer the backlog, and Service Bus reports it")
	}
	if _, degraded := real.DegradedReason(model.CapSubscriptionLag); degraded {
		t.Error("a real namespace degrades the backlog it can answer")
	}

	emulator := (&Conn{
		closed: make(chan struct{}),
		config: clientConfig{namespace: "localhost:5672", emulatorManagement: "127.0.0.1:5300"},
	}).declare()
	if emulator.Has(model.CapSubscriptionLag) {
		t.Error("an emulator offers the backlog as a figure, and it reports none")
	}
	reason, degraded := emulator.DegradedReason(model.CapSubscriptionLag)
	if !degraded {
		t.Fatal("an emulator neither supports nor explains the backlog")
	}
	if reason != countsNotInEmulator {
		t.Errorf("reason = %q, want %q", reason, countsNotInEmulator)
	}
	// Every other capability survives the narrowing: only the one figure goes.
	for _, capability := range capabilities() {
		if capability == model.CapSubscriptionLag {
			continue
		}
		if !emulator.Has(capability) {
			t.Errorf("%s was lost along with the backlog", capability)
		}
	}
}
