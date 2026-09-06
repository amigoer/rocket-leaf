package app

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	pubsubdriver "github.com/amigoer/mq-studio/internal/driver/googlepubsub"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/destination"
	pubsubservice "github.com/amigoer/mq-studio/internal/service/googlepubsub"
	"github.com/amigoer/mq-studio/internal/service/message"
	"github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/service/subscription"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

/*
 * The Google Pub/Sub stack from the outside, through a connection id.
 *
 * The driver's own live tests hold a *Conn. These hold nothing: they store a
 * profile, dial it, and then ask the service layer the way a page does - with
 * an integer. What that covers and the driver's tests do not is the chain in
 * between, where the two failures this project has actually had both lived: a
 * value that did not survive being written to disk and read back, and a
 * capability the service checks before the type assertion.
 *
 * On this family the first risk is the same one SQS carried and one field
 * larger. A Pub/Sub profile has no address at all, so what has to survive the
 * round trip is a project, a service account key that is a whole JSON
 * document, a name prefix and an emulator host - and getting any of them wrong
 * produces a profile that saves, reloads and then cannot dial.
 */

const (
	livePubSubEmulator = "127.0.0.1:8085"
	livePubSubProject  = "mq-studio-e2e"
)

// The objects scripts/e2e-google-pubsub-seed.sh creates.
const (
	livePubSubOrders      = "mqs-seed-orders"
	livePubSubDeadLetters = "mqs-seed-dead-letters"
	livePubSubOrphaned    = "mqs-seed-orphaned"
	livePubSubQuiet       = "mqs-seed-quiet"

	livePubSubWorker     = "mqs-seed-orders-worker"
	livePubSubAudit      = "mqs-seed-orders-audit"
	livePubSubDeadReader = "mqs-seed-dead-letters-reader"
	livePubSubIdle       = "mqs-seed-quiet-idle"
)

func requireLivePubSub(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.GooglePubSub,
		Name:   "the google pub/sub e2e environment",
		Start:  "npm run e2e:google-pubsub:up",
		Probe: e2e.HTTPGet(
			"http://" + livePubSubEmulator + "/v1/projects/" + livePubSubProject + "/topics"),
	})
}

// pubsubStack is the connection service, the Pub/Sub service and the canonical
// services a board reads through, on a config directory of its own.
type pubsubStack struct {
	connections   *connection.Service
	pubsub        *pubsubservice.Service
	destinations  *destination.Service
	subscriptions *subscription.Service
	messages      *message.Service
	// conns is what the bridge holds to answer capability questions without
	// going through a domain service, which is how the sidebar decides what to
	// draw and why.
	conns func(connID int) (driver.Conn, error)
	// dataFile is where the profiles land, so a second service can be opened
	// on the same store and prove what actually survived disk.
	dataFile string
	settings *settings.Service
}

func newPubSubStack(t *testing.T) *pubsubStack {
	t.Helper()
	if _, ok := driver.Lookup(model.KindGooglePubSub); !ok {
		driver.Register(pubsubdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	conns := newConnSource(registry)
	return &pubsubStack{
		connections: connection.New(
			paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints()),
		pubsub:        pubsubservice.New(conns, settingsService),
		destinations:  destination.New(conns, settingsService),
		subscriptions: subscription.New(conns, settingsService),
		messages:      message.New(conns, settingsService),
		conns:         conns,
		dataFile:      paths.ConnectionsFile,
		settings:      settingsService,
	}
}

// livePubSubProfile is the environment as a user would configure it: a
// project, an emulator host, and no address anywhere on the form.
func livePubSubProfile(name string) model.ConnectionProfile {
	return model.ConnectionProfile{
		Name:       name,
		Kind:       model.KindGooglePubSub,
		TimeoutSec: 10,
		Auth:       model.AuthConfig{Mechanism: model.AuthNone},
		Options: map[string]string{
			pubsubdriver.OptionProjectID:    livePubSubProject,
			pubsubdriver.OptionEmulatorHost: livePubSubEmulator,
		},
	}
}

// dial stores a profile and opens it, returning the id a page would hold.
func (s *pubsubStack) dial(t *testing.T, profile model.ConnectionProfile) int {
	t.Helper()
	created, err := s.connections.AddConnection(profile)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	if err := s.connections.Connect(created.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = s.connections.Disconnect(created.ID) })
	return created.ID
}

func pubsubContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// testPubSubTopicVia creates a topic through the service layer and removes it
// when the test ends, so nothing a run leaves behind changes the next one.
func testPubSubTopicVia(t *testing.T, stack *pubsubStack, connID int, name string) {
	t.Helper()
	if err := stack.pubsub.CreateTopic(pubsubContext(t), connID,
		pubsubdriver.TopicSpec{Name: name}); err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = stack.pubsub.RemoveTopic(context.Background(), connID, name)
	})
}

func testPubSubSubscriptionVia(
	t *testing.T, stack *pubsubStack, connID int, spec pubsubdriver.SubscriptionSpec,
) {
	t.Helper()
	if err := stack.pubsub.CreateSubscription(pubsubContext(t), connID, spec); err != nil {
		t.Fatalf("creating %s: %v", spec.Name, err)
	}
	t.Cleanup(func() {
		_ = stack.pubsub.RemoveSubscription(context.Background(), connID, spec.Name)
	})
}

func pubsubTestName(t *testing.T, suffix string) string {
	t.Helper()
	return "mqs-test-app-" + strconv.FormatInt(time.Now().UnixNano()%1e9, 36) + suffix
}

/*
 * The whole path in one go: store a profile with no address, dial it, declare
 * a topic and a subscription, publish, and read all three back through the id
 * a page would pass.
 */
func TestLivePubSubStackRoundTrip(t *testing.T) {
	requireLivePubSub(t)
	stack := newPubSubStack(t)
	connID := stack.dial(t, livePubSubProfile("pubsub stack"))
	topic := pubsubTestName(t, "-topic")
	reader := pubsubTestName(t, "-reader")

	testPubSubTopicVia(t, stack, connID, topic)
	testPubSubSubscriptionVia(t, stack, connID, pubsubdriver.SubscriptionSpec{
		Name: reader, Topic: topic, AckDeadlineSec: 60,
	})

	result, err := stack.pubsub.Publish(pubsubContext(t), connID, pubsubdriver.PublishRequest{
		Topic: topic, Body: "through the stack", Count: 4,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.Sent != 4 {
		t.Fatalf("sent %d, want 4", result.Sent)
	}

	destinations, err := stack.destinations.List(pubsubContext(t), connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	var listed *model.Destination
	for _, entry := range destinations {
		if entry.Ref.Name == topic {
			listed = entry
		}
	}
	if listed == nil {
		t.Fatal("the topic the stack created is not in the listing")
	}
	// The figure this family leads with, and the one a topic can actually
	// answer: how many subscriptions read it.
	if listed.Subscribers != 1 {
		t.Errorf("subscribers = %d, want the one subscription created", listed.Subscribers)
	}
	if listed.Depth != model.UnknownMetric {
		t.Errorf("depth = %d; a topic holds nothing countable", listed.Depth)
	}

	groups, err := stack.subscriptions.List(pubsubContext(t), connID)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	var found *model.Subscription
	for _, entry := range groups {
		if entry.Ref.Name == reader {
			found = entry
		}
	}
	if found == nil {
		t.Fatal("the subscription the stack created is not in the listing")
	}
	if found.Attribute("topic") != topic {
		t.Errorf("the subscription reads %q, want %q", found.Attribute("topic"), topic)
	}

	// The browse takes a subscription where every other family takes a
	// destination, which is the one place this family's vocabulary crosses the
	// canonical service.
	messages, err := stack.messages.Query(pubsubContext(t), connID, model.MessageQueryParams{
		Topic: reader, MaxResults: 4,
	})
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if len(messages) != 4 {
		t.Errorf("browsed %d messages, want the 4 published", len(messages))
	}

	// The renderer's list keys, which the service assigns rather than the
	// driver: a page keys its rows on them, and duplicates make React drop
	// rows silently.
	seen := map[int]bool{}
	for _, entry := range destinations {
		if seen[entry.ID] {
			t.Errorf("two destinations share the list key %d", entry.ID)
		}
		seen[entry.ID] = true
	}
}

/*
 * The addressless profile, proved through disk.
 *
 * This is the same thing phase 13 proved for SQS and one field harder. A
 * profile whose Endpoints is empty has to save - the connection service
 * refuses one that is empty for every other family - come back off disk with
 * its project, its name prefix, its emulator host and a service account key
 * that is a multi-line JSON document intact, and then dial. Losing any one of
 * them produces a profile that saves and reloads and cannot connect, which is
 * the shape of failure this family is most exposed to: there is no address to
 * look at and notice missing.
 */
func TestLivePubSubAddresslessProfileSurvivesDisk(t *testing.T) {
	requireLivePubSub(t)
	stack := newPubSubStack(t)

	// A service account key as a user would paste one: real JSON, several
	// lines, and an embedded private key whose newlines are load-bearing.
	const key = "{\n  \"type\": \"service_account\",\n  \"project_id\": \"mq-studio-e2e\",\n" +
		"  \"private_key\": \"-----BEGIN PRIVATE KEY-----\\nMIIB\\n-----END PRIVATE KEY-----\\n\"\n}"

	input := livePubSubProfile("pubsub from disk")
	input.Options[pubsubdriver.OptionResourcePrefix] = "mqs-seed-"
	input.Auth = model.AuthConfig{Mechanism: model.AuthPlain}
	input.SetSecret(pubsubdriver.SecretCredentialsJSON, key)

	created, err := stack.connections.AddConnection(input)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	if created.Endpoints != "" {
		t.Fatalf("the saved profile carries an address %q; this family has none", created.Endpoints)
	}

	// A second service on the same file, which is what a restart is.
	reopened := connection.New(
		stack.dataFile, stack.settings, newRegistryRuntime(driver.NewRegistry()), newDescriptorEndpoints())
	stored, err := reopened.GetConnection(created.ID)
	if err != nil {
		t.Fatalf("read the stored profile: %v", err)
	}
	if stored.Endpoints != "" {
		t.Errorf("the stored profile grew an address %q", stored.Endpoints)
	}
	if stored.Option(pubsubdriver.OptionProjectID) != livePubSubProject {
		t.Errorf("project after reload = %q, want %q",
			stored.Option(pubsubdriver.OptionProjectID), livePubSubProject)
	}
	if stored.Option(pubsubdriver.OptionEmulatorHost) != livePubSubEmulator {
		t.Errorf("emulator host after reload = %q, want %q",
			stored.Option(pubsubdriver.OptionEmulatorHost), livePubSubEmulator)
	}
	if stored.Option(pubsubdriver.OptionResourcePrefix) != "mqs-seed-" {
		t.Errorf("name prefix after reload = %q", stored.Option(pubsubdriver.OptionResourcePrefix))
	}
	// Byte for byte, because a key is not a short string: the newlines inside
	// the private key are part of it, and a store that trimmed or re-wrapped
	// them would produce a credential that saves and cannot sign.
	if stored.Secret(pubsubdriver.SecretCredentialsJSON) != key {
		t.Errorf("the service account key did not survive disk intact:\n%q",
			stored.Secret(pubsubdriver.SecretCredentialsJSON))
	}
	/*
	 * The reserved names, which are the trap this family walks past. accessKey
	 * and secretKey are RocketMQ's ACL pair: they skip applyCredentials'
	 * generic loop, are written only through SetACL, and are filled from
	 * global settings for a profile that named no mechanism. A family reusing
	 * them would have its own credential cleared on save and RocketMQ's global
	 * pair stamped on at dial time.
	 */
	if stored.Secret(model.SecretAccessKey) != "" || stored.Secret(model.SecretSecretKey) != "" {
		t.Error("the reserved RocketMQ ACL names were written onto a Pub/Sub profile")
	}
	if stored.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism after reload = %q, want plain", stored.Auth.Mechanism)
	}

	// And it dials, which is the half a stored-value check cannot cover. The
	// emulator ignores the key rather than checking it, so what this proves is
	// that carrying one does not stop the connection working.
	if err := stack.connections.Connect(created.ID); err != nil {
		t.Fatalf("connect a profile read back from disk: %v", err)
	}
	t.Cleanup(func() { _ = stack.connections.Disconnect(created.ID) })

	destinations, err := stack.destinations.List(pubsubContext(t), created.ID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	if len(destinations) == 0 {
		e2e.Missing(t, "the project holds no mqs-seed- topics; run `npm run e2e:google-pubsub:seed`")
	}
	// The prefix survived too, which is only visible in what the listing did
	// not return.
	for _, entry := range destinations {
		if !strings.HasPrefix(entry.Ref.Name, "mqs-seed-") {
			t.Errorf("%s is outside the stored prefix, which did not survive disk", entry.Ref.Name)
		}
	}
}

/*
 * The capability set a real connection reports, which is what the sidebar
 * draws from.
 *
 * Asserted here rather than only in the driver's package because this is the
 * object a page actually holds: the driver's test builds a Conn by hand, and
 * this one comes back from a dial through the registry.
 */
func TestLivePubSubCapabilitiesReachThePages(t *testing.T) {
	requireLivePubSub(t)
	stack := newPubSubStack(t)
	connID := stack.dial(t, livePubSubProfile("pubsub capabilities"))

	conn, err := stack.conns(connID)
	if err != nil {
		t.Fatalf("resolve the connection: %v", err)
	}
	capabilities := conn.Capabilities()

	for _, capability := range []model.Capability{
		model.CapDestinationList,
		model.CapSubscriptionList,
		model.CapSubscriptionPosition,
		model.CapMessageQuery,
		model.CapPublish,
		model.CapDeadLetterTopology,
	} {
		if !capabilities.Has(capability) {
			t.Errorf("a live connection does not report %s, so its page is not drawn", capability)
		}
	}

	// The one every Pub/Sub connection degrades, whatever it is pointed at:
	// the backlog is a Cloud Monitoring metric and this API reports none.
	reason, degraded := capabilities.DegradedReason(model.CapSubscriptionLag)
	if !degraded {
		t.Error("the backlog is not explained, so the subscriptions page says nothing about it")
	}
	if reason != "mq.google-pubsub.degraded.lagInMonitoring" {
		t.Errorf("degraded reason = %q, want the key the renderer resolves", reason)
	}

	// The caveat the messages page draws before its button, which is what
	// makes the difference between a page that works and one that cannot be
	// opened.
	caveat, warned := capabilities.Caveat(model.CapMessageQuery)
	if !warned || caveat != "mq.google-pubsub.caveat.pullDelivers" {
		t.Errorf("browsing carries caveat %q, want the key the renderer resolves", caveat)
	}

	// And what it must not claim. Google runs the service, so there is no
	// node, no session and no ACL this connection could show.
	for _, capability := range []model.Capability{
		model.CapClusterTopology,
		model.CapClusterMetrics,
		model.CapClientInspect,
		model.CapAccessControl,
		model.CapNamespaceList,
	} {
		if capabilities.Has(capability) {
			t.Errorf("a live connection reports %s, and Google runs this service", capability)
		}
	}
}
