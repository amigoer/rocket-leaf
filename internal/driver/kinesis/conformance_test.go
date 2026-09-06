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

/*
 * The backlog is degraded rather than absent or filled in, and this is why.
 *
 * A registered consumer is a real object - it can be listed, created and
 * removed - so the family plainly has subscriptions, and hiding the backlog
 * column outright would leave a reader waiting for a number that is never
 * coming. What it does not have is a position: a classic consumer keeps one in
 * a DynamoDB table the KCL owns, and an enhanced fan-out consumer keeps none
 * at all.
 *
 * The thing that must not happen is the number being invented from
 * MillisBehindLatest, which is how far behind the tip one GetRecords call was.
 * That belongs to whoever made the call - this app, when it browses - and
 * says nothing about any consumer.
 */
func TestSubscriptionLagIsDegradedRatherThanInvented(t *testing.T) {
	declared := (&Conn{closed: make(chan struct{})}).declare()

	if declared.Has(model.CapSubscriptionLag) {
		t.Fatal("declares a subscription backlog, and no call in the API returns one")
	}
	reason, degraded := declared.DegradedReason(model.CapSubscriptionLag)
	if !degraded {
		t.Fatal("the backlog is absent with no reason; a consumers page would show a " +
			"column that never arrives and say nothing about why")
	}
	if reason != positionInDynamo {
		t.Errorf("reason = %q, want %q", reason, positionInDynamo)
	}
	if !declared.Has(model.CapSubscriptionList) {
		t.Error("degrades the backlog of subscriptions it does not declare")
	}
}

/*
 * The degraded reasons cross a language boundary as keys, the same as the
 * caveats above, and the renderer's own key test cannot see either.
 */
func TestDegradedReasonsAreTranslationKeys(t *testing.T) {
	for _, reason := range []string{positionInDynamo} {
		if !isTranslationKey(reason) {
			t.Errorf("%q is a sentence, not an i18n key", reason)
		}
	}
}

/*
 * What Kinesis has no concept of, and why.
 *
 * Every entry is a capability another family in this app declares and this one
 * must not, and each reason is about Kinesis rather than about how far the
 * driver has got. Without this list the cheapest way to add a family is to
 * copy a neighbour's capability set, and the result is a sidebar full of pages
 * that open onto nothing.
 *
 * Three roots cover nearly all of it. Nothing is ever moved aside - a record
 * stays where it was written until retention expires, read or not - so every
 * dead-letter, retry and pending shape is absent. Nothing keeps a reader's
 * position: a classic consumer stores one in DynamoDB and a registered one
 * stores none, so nothing about progress can be read or written. And AWS runs
 * the service, so there is no node, no process and no setting an operator here
 * could reach.
 */
func TestConnDeclaresNoConceptKinesisDoesNotHave(t *testing.T) {
	absent := []struct {
		capability model.Capability
		because    string
	}{
		{
			model.CapPartitions,
			"a stream is divided into shards, and a shard is not a partition number. " +
				"It has an id, a hash key range that decides which records land on it, a " +
				"read quota of its own, and a parent it was split from - none of which " +
				"survives a map keyed by an index, which is what DestinationStats answers. " +
				"The concept has a port and a capability of its own instead.",
		},
		{
			model.CapSubscriptionLag,
			"declared as degraded rather than absent, which the backlog test above " +
				"pins: the family has subscriptions and no reader position anywhere.",
		},
		{
			model.CapOffsetReset,
			"there is no position to move. A reader holds a shard iterator, which is a " +
				"cursor it created and nobody else can see, and the KCL's checkpoint is a " +
				"row in a DynamoDB table this connection is not signed for.",
		},
		{
			model.CapSubscriptionPosition,
			"and nothing that names a place for a subscription to be moved to. A " +
				"sequence number addresses a record within one shard; there is no call " +
				"that points a consumer at one.",
		},
		{
			model.CapQueueOffset,
			"per shard makes it no more possible: the position is the reader's, wherever " +
				"it chose to keep it.",
		},
		{
			model.CapOffsetClone,
			"and with no stored position there is nothing to copy from one reader onto " +
				"another.",
		},
		{
			model.CapSubscriptionRuntime,
			"a registered consumer is a registration rather than a connection. The " +
				"service reports that it exists and never reports whether anything is " +
				"attached to it, so there is no member to ask what it is working on.",
		},
		{
			model.CapDLQ,
			"nothing is ever moved aside. A record stays on its shard until the " +
				"retention period expires whether it was read once, many times or not at " +
				"all, so there is no dead-letter store to browse.",
		},
		{
			model.CapDeadLetterTopology,
			"and no topology pointing at one either. A stream references no other " +
				"stream, so there is nothing to walk backwards.",
		},
		{
			model.CapMessageResend,
			"a retry is the reader's own business: it re-reads from a sequence number " +
				"it kept. Nothing is put back, because nothing was taken.",
		},
		{
			model.CapMessageReplay,
			"and there is no connected consumer to hand a record to. The service knows " +
				"a consumer is registered and nothing about who is running it.",
		},
		{
			model.CapPendingEntries,
			"nothing is owed. A read hands out a copy and records nothing, so there is " +
				"no delivery to be outstanding and no list of what has not been " +
				"acknowledged.",
		},
		{
			model.CapPendingAdmin,
			"and with no such list there is nothing to acknowledge or reassign.",
		},
		{
			model.CapMessageTrack,
			"there is no trace. A record carries what the sender wrote and the sequence " +
				"number the service assigned, and nothing about who has read it.",
		},
		{
			model.CapMessageLiveTail,
			"not offered, and it is the one absence here that is this driver's rather " +
				"than the service's: a shard iterator is exactly the cursor a tail needs, " +
				"and following a stream costs five GetRecords a second per shard shared " +
				"with its consumers. It is left out until there is a page that spends that " +
				"deliberately rather than as a background poll.",
		},
		{
			model.CapLiveStream,
			"nothing is pushed to a caller that did not ask. SubscribeToShard streams to " +
				"one registered consumer over HTTP/2, which is a reader this app would " +
				"have to become rather than a broker pushing at it.",
		},
		{
			model.CapDelayedDelivery,
			"nothing holds a record back. A record is readable the moment the service " +
				"accepts it, and there is no scheduled send anywhere in the API.",
		},
		{
			model.CapPublishRich,
			"a record is bytes, a partition key and an optional hash key. There is no " +
				"exchange, no routing key, no header table and no delivery mode for a " +
				"richer console to collect.",
		},
		{
			model.CapEntryPublish,
			"nor is a record an ordered set of named fields. What is inside the bytes is " +
				"the sender's business and the service never looks.",
		},
		{
			model.CapProducerInspect,
			"nothing is connected. A producer is whoever signs a PutRecord, and the " +
				"service keeps no record of who that was.",
		},
		{
			model.CapDestinationPurge,
			"a stream cannot be emptied. Retention is the only thing that removes a " +
				"record, and lowering it is a setting on the stream rather than an " +
				"operation on its contents - which is why it is on the edit form.",
		},
		{
			model.CapStreamTrim,
			"and there is no trim either. A trim names a bound to keep and removes the " +
				"rest; the retention period is a duration the service enforces on its own " +
				"schedule, and nothing here chooses which records go.",
		},
		{
			model.CapDestinationMove,
			"nothing drains one stream into another. A record is read by whoever wants " +
				"it and stays where it is.",
		},
		{
			model.CapQueueRebalance,
			"placement is AWS's. Shards are spread across the service's own capacity and " +
				"there is no node here to spread them over.",
		},
		{
			model.CapReassign,
			"and a shard has no replica list. How many copies the service keeps is not " +
				"reported anywhere and is not an administrator's to edit.",
		},
		{
			model.CapClusterTopology,
			"there is no cluster. Kinesis is a regional service with no node, address or " +
				"process an operator here could be shown, and a topology board would have " +
				"exactly one invented row on it.",
		},
		{
			model.CapClusterMetrics,
			"and no node to attribute a figure to. What the service reports is per " +
				"stream, which the streams board already shows; the rates live in " +
				"CloudWatch, a different service with a different credential.",
		},
		{
			model.CapDirectory,
			"there is no discovery tier. A region resolves to an endpoint by name, and " +
				"nothing has to be asked where a stream lives.",
		},
		{
			model.CapNodeConfig,
			"a stream's settings are already on its own page, and there is no node " +
				"underneath with settings of its own.",
		},
		{
			model.CapNodeMaintenance,
			"nothing here is maintained by its user. Retention is a stream setting AWS " +
				"enforces on its own schedule.",
		},
		{
			model.CapNodeWritePerm,
			"and nothing can be taken out of the write path. A shard closes by being " +
				"split or merged, which is a capacity change rather than a drain.",
		},
		{
			model.CapLogDirs,
			"AWS reports no storage figure at all - not size, not free space, not a " +
				"percentage. A stream is billed by shard hour and payload, not by what it " +
				"is holding.",
		},
		{
			model.CapSlowLog,
			"nothing records what has been slow. Latency is a CloudWatch metric, and it " +
				"is an aggregate rather than a request.",
		},
		{
			model.CapClusterHealth,
			"Kinesis answers no question about itself. Service health is the AWS Health " +
				"Dashboard, which is a different API this connection is not signed for.",
		},
		{
			model.CapClusterCensus,
			"there is no account-wide total. Every figure the service reports is one " +
				"stream's, and summing them would mean a request per stream and a number " +
				"that was never true at any single moment.",
		},
		{
			model.CapClientInspect,
			"nothing holds a connection. Every call is a signed HTTPS request that stands " +
				"alone, so there is no session to list.",
		},
		{
			model.CapClientClose,
			"and nothing to disconnect for the same reason.",
		},
		{
			model.CapAccessControl,
			"access is IAM's, not the stream's. A stream carries a resource policy, but " +
				"who may call what is decided by identities in a service this connection " +
				"is not signed for - and a page editing half of that would claim to " +
				"control access it cannot see.",
		},
		{
			model.CapAccessDirectory,
			"same reason: the directory of principals is IAM, one service further out.",
		},
		{
			model.CapAclUsers,
			"and Kinesis keeps no users of its own to attach rules to.",
		},
		{
			model.CapNamespaceList,
			"a stream name is flat and unique within an account and region. There is no " +
				"tenant, vhost or account inside Kinesis for one to live in.",
		},
		{
			model.CapTransactions,
			"a send is one record, or a batch of five hundred that succeed and fail " +
				"individually. Nothing spans shards, so there is no transaction with an " +
				"identity to list.",
		},
		{
			model.CapQuotaList,
			"the limits are the service's own and are the same for every caller. They " +
				"are per shard and per account rather than per identity, cannot be read " +
				"back through this API, and nothing here could change one.",
		},
		{
			model.CapRouting,
			"there is no exchange and no binding. Which shard takes a record is decided " +
				"by hashing its partition key, which is arithmetic rather than a topology " +
				"anybody configured.",
		},
		{
			model.CapConnectionScope,
			"the stream prefix on this form filters a listing; it does not re-point the " +
				"connection. A name outside it is still perfectly reachable, which is not " +
				"what a scope means anywhere else in this app.",
		},
	}

	live := offlineConn().Capabilities()
	for _, entry := range absent {
		if live.Has(entry.capability) {
			t.Errorf("declares %s, but %s", entry.capability, entry.because)
		}
		// The backlog is the one entry that is deliberately degraded, and its
		// own test asserts the reason - so only the others must not be.
		if entry.capability == model.CapSubscriptionLag {
			continue
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
 * The list below is the one frontend/src/mq/navigation.kinesis.test.ts holds,
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
		"destination.update",
		"destination.delete",
		"destination.shards",
		"message.query",
		"message.byId",
		"message.publish",
		"subscription.list",
		"subscription.create",
		"subscription.delete",
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
				"restore it or drop the page, and update navigation.kinesis.test.ts in the same commit",
				capability)
		}
	}
	for capability := range declared {
		if !expected[capability] {
			t.Errorf("the driver declares %s and the sidebar contract does not list it; "+
				"add it to navigation.kinesis.test.ts in the same commit", capability)
		}
	}
}
