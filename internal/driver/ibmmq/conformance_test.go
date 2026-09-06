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

/*
 * What IBM MQ has no concept of, and why.
 *
 * Every entry is a capability another family in this app declares and this one
 * must not, and each reason is about IBM MQ rather than about how far the
 * driver has got - except where it says otherwise. Without this list the
 * cheapest way to add a family is to copy a neighbour's capability set, and
 * the result is a sidebar full of pages that open onto nothing.
 *
 * Three roots cover most of it. A queue manager stores messages and does not
 * track who read them, so nothing about a reader's position exists anywhere.
 * There is one queue manager rather than a cluster of brokers, so every
 * node-shaped figure has one row to put on it and no node to attribute it to.
 * And the management plane is HTTP, so what is missing is what those two REST
 * interfaces do not carry rather than what the product cannot do - which is
 * why a few of these are this driver's own and say so.
 */
func TestConnDeclaresNoConceptIBMMQDoesNotHave(t *testing.T) {
	absent := []struct {
		capability model.Capability
		because    string
	}{
		{
			model.CapPartitions,
			"IBM MQ divides nothing. A queue is one store on one queue manager, and a " +
				"cluster queue is several queues that share a name rather than one queue in " +
				"parts, so a partition count would be a number with nothing behind it.",
		},
		{
			model.CapShards,
			"and there is no shard either: nothing here is split by a hash of a key.",
		},
		{
			model.CapClientInspect,
			"declared as CapChannels instead, which is not the same page. This connection " +
				"can list the channel definitions everything has to come through; what it " +
				"cannot enumerate is the transport connections open right now.",
		},
		{
			model.CapClientClose,
			"and with no connection list there is nothing to disconnect. STOP CONNECTION " +
				"exists in MQSC and needs a connection identifier this driver never sees.",
		},
		{
			model.CapOffsetReset,
			"there is no position to move. A queue manager hands a message to whoever gets " +
				"it and forgets; nothing anywhere records how far a reader has got.",
		},
		{
			model.CapQueueOffset,
			"per queue makes it no more possible, for the same reason.",
		},
		{
			model.CapOffsetClone,
			"and with no stored position there is nothing to copy from one reader onto " +
				"another.",
		},
		{
			model.CapSubscriptionPosition,
			"nor is there a place in a log to name. A queue is not a log: a message that " +
				"has been read is gone, so there is nothing to rewind to.",
		},
		{
			model.CapSubscriptionRuntime,
			"a subscription reports at most one attached connection and an identifier for " +
				"it, and nothing about what that application is doing with it.",
		},
		{
			model.CapSubscriptionCreate,
			"this driver's omission rather than the family's, and the only one on this " +
				"list: DEFINE SUB works through the same endpoint. A subscription's " +
				"identity is a topic string, a destination queue and a durability together, " +
				"and that needs a form rather than a name field.",
		},
		{
			model.CapSubscriptionDelete,
			"left out with the create above, for the same reason.",
		},
		{
			model.CapDestinationUpdate,
			"also this driver's, and also deliberate. ALTER changes a live object " +
				"underneath whatever has it open, and the fields worth changing each have " +
				"their own consequence for applications already connected - so this driver " +
				"reads them and offers no one control that writes them all.",
		},
		{
			model.CapDLQ,
			"declared as CapDeadLetterTopology instead. Nothing here is a dead-letter " +
				"queue by nature: what makes one is the queue manager's DEADQ attribute or " +
				"another queue's backout queue pointing at it, which is a walk backwards " +
				"rather than a name derived from a group.",
		},
		{
			model.CapMessageResend,
			"nothing puts a dead letter back. Moving one is an application's job - the " +
				"dead-letter header says where it was going, and deciding what to do about " +
				"that is a policy rather than a button.",
		},
		{
			model.CapMessageReplay,
			"and there is no connected consumer to hand a message to. A queue manager " +
				"knows an application has a queue open and nothing about its handler.",
		},
		{
			model.CapPendingEntries,
			"nothing is owed. A message is either on the queue or has been taken; there " +
				"is no delivery record, and an uncommitted get is a transaction rather " +
				"than a list.",
		},
		{
			model.CapPendingAdmin,
			"and with no such list there is nothing to acknowledge or reassign.",
		},
		{
			model.CapMessageTrack,
			"there is no trace on an ordinary message. Activity recording exists and is a " +
				"separate feature that writes its own messages to a system queue, which is " +
				"not what this page asks for.",
		},
		{
			model.CapMessageLiveTail,
			"a queue is not a log. A tail is an incremental read against a durable " +
				"position, and there is no position here to be incremental against - the " +
				"only way to follow a queue is to browse it again.",
		},
		{
			model.CapLiveStream,
			"and nothing is pushed to a caller that did not ask. The REST interfaces are " +
				"request and response; following a subscription means being an MQ client.",
		},
		{
			model.CapDelayedDelivery,
			"nothing holds a message back. A message the queue manager accepts is " +
				"readable immediately; the delayed-delivery capability of MQ's own " +
				"messaging APIs is the client library's timer rather than the broker's.",
		},
		{
			model.CapPublishRich,
			"and there is nowhere richer to send from. The messaging interface has no " +
				"topic endpoint at all, so a console with an exchange and a routing key " +
				"would be collecting fields that cannot be sent.",
		},
		{
			model.CapEntryPublish,
			"a message is bytes with a descriptor, not an ordered set of named fields.",
		},
		{
			model.CapProducerInspect,
			"there is no producer group. An application that puts a message opens a queue " +
				"and closes it, and the queue manager counts the open handles rather than " +
				"naming who holds them.",
		},
		{
			model.CapDestinationPurge,
			"this driver's own, and narrowly. CLEAR QLOCAL exists; what is offered here is " +
				"the purge that goes with a delete, because emptying a queue somebody else " +
				"is using is a larger gesture than it looks and deserves its own page.",
		},
		{
			model.CapDestinationMove,
			"nothing drains one queue into another. A queue manager moves messages along " +
				"channels, which is a route rather than an operation on contents.",
		},
		{
			model.CapStreamTrim,
			"and there is no trim: a queue holds what it holds until something reads it " +
				"or its expiry passes.",
		},
		{
			model.CapQueueRebalance,
			"placement is not a thing here. A queue lives on the queue manager it was " +
				"defined on, and there is nowhere else to put it.",
		},
		{
			model.CapReassign,
			"and a queue has no replica list. High availability is the queue manager's, " +
				"configured outside it, and not an administrator's to edit per queue.",
		},
		{
			model.CapClusterTopology,
			"there is no cluster under this connection. An IBM MQ cluster is a set of " +
				"queue managers publishing to each other's repositories; this connection " +
				"speaks to one of them, so a topology board would have one invented row.",
		},
		{
			model.CapClusterMetrics,
			"and no node to attribute a figure to for the same reason.",
		},
		{
			model.CapClusterCensus,
			"the queue manager keeps no running total of its own. Every figure it reports " +
				"belongs to one object, and summing them would be this app's arithmetic " +
				"rather than the queue manager's answer.",
		},
		{
			model.CapClusterHealth,
			"nothing answers a question about the queue manager's health. Its state is " +
				"running or not, which is what the connection already reports.",
		},
		{
			model.CapDirectory,
			"there is no discovery tier. A client is given an address and a channel name, " +
				"and nothing has to be asked where a queue manager is.",
		},
		{
			model.CapNodeConfig,
			"the queue manager's own settings are readable and are one object's " +
				"attributes rather than a node's effective configuration; there is no node " +
				"underneath it with settings of its own.",
		},
		{
			model.CapNodeMaintenance,
			"nothing here is run on demand through this interface. A queue manager's " +
				"housekeeping is its log and its media images, which are commands on the " +
				"machine it runs on.",
		},
		{
			model.CapNodeWritePerm,
			"and there is no node to take out of the write path. Inhibiting a queue is a " +
				"queue's attribute rather than a broker being drained.",
		},
		{
			model.CapLogDirs,
			"the REST interfaces report no storage figure at all - not a size, not free " +
				"space, not a percentage. What a queue manager's log occupies is readable " +
				"on the machine it runs on.",
		},
		{
			model.CapSlowLog,
			"nothing records what has been slow. A queue's monitoring data reports how " +
				"long messages sat on it, which is a different question and belongs to the " +
				"queue rather than to a command.",
		},
		{
			model.CapAccessControl,
			"access here is an authority record per object and per principal, which is " +
				"neither a credential pair the broker takes a write for nor a rule attached " +
				"to a subject. It is a page of its own rather than a column, and this " +
				"driver does not draw it.",
		},
		{
			model.CapAccessDirectory,
			"and there is no directory of principals inside the queue manager: it " +
				"authorises operating system users and groups, or an LDAP repository, both " +
				"of which live outside it.",
		},
		{
			model.CapAclUsers,
			"nor does it keep users of its own to attach rules to.",
		},
		{
			model.CapIdentityList,
			"same reason: the identities are the machine's or a directory's.",
		},
		{
			model.CapNamespaceList,
			"a queue name is flat and unique within its queue manager. There is no vhost, " +
				"tenant or namespace inside one - a second queue manager is a second " +
				"connection rather than a namespace in this one.",
		},
		{
			model.CapConnectionScope,
			"and the queue manager is not a scope either. A scope is a naming convention " +
				"with an unscoped default; a queue manager is a separate process with its " +
				"own storage and log, nothing crosses between two of them, and there is no " +
				"unscoped IBM MQ connection at all.",
		},
		{
			model.CapRouting,
			"there is no exchange and no binding. Where a message goes is decided by the " +
				"name it was put to and, for a publication, by the topic tree - which is a " +
				"hierarchy rather than a topology anybody wired up.",
		},
		{
			model.CapPolicyList,
			"and nothing is applied to queues by pattern. A queue's settings are its own, " +
				"inherited from the model queue it was copied from at definition time.",
		},
		{
			model.CapQuotaList,
			"limits here are per object - a queue's maximum depth, a channel's message " +
				"size - rather than per identity. There is nothing that throttles one " +
				"application across the queue manager.",
		},
		{
			model.CapTransactions,
			"a queue manager coordinates transactions and does not publish them as a " +
				"list. An in-doubt unit of work belongs to a channel, which is on the " +
				"channels page and reported there as what it is.",
		},
		{
			model.CapDefinitionsExport,
			"there is no one document. A queue manager's configuration is dumped by " +
				"running MQSC on the machine it lives on, which is a command rather than " +
				"an object this connection can ask for.",
		},
		{
			model.CapReplication,
			"nothing moves messages between queue managers on this connection's behalf. " +
				"That is what channels do, and they are a topology rather than a shovel " +
				"this app could create.",
		},
		{
			model.CapStreamClients,
			"and there is no second protocol to enumerate readers of. The AMQP channel " +
				"exists and is a channel like any other, which is where it is reported.",
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
 * The sidebar contract, from the Go side.
 *
 * The list below is the one frontend/src/mq/navigation.ibmmq.test.ts holds,
 * and that test asserts which pages those capabilities make reachable. This
 * one asserts the driver still declares exactly them.
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
		"destination.delete",
		"channel.list",
		"message.query",
		"message.byId",
		"message.publish",
		"message.dlqTopology",
		"subscription.list",
		"subscription.lag",
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
				"restore it or drop the page, and update navigation.ibmmq.test.ts in the same commit",
				capability)
		}
	}
	for capability := range declared {
		if !expected[capability] {
			t.Errorf("the driver declares %s and the sidebar contract does not list it; "+
				"add it to navigation.ibmmq.test.ts in the same commit", capability)
		}
	}
}
