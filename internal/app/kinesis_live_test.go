package app

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	kinesisdriver "github.com/amigoer/mq-studio/internal/driver/kinesis"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/destination"
	kinesisservice "github.com/amigoer/mq-studio/internal/service/kinesis"
	"github.com/amigoer/mq-studio/internal/service/message"
	"github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/service/subscription"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

/*
 * The Amazon Kinesis stack from the outside, through a connection id.
 *
 * The driver's own live tests hold a *Conn. These hold nothing: they store a
 * profile, dial it, and then ask the service layer the way a page does - with
 * an integer. What that covers and the driver's tests do not is the chain in
 * between, where the two failures this project has actually had both lived: a
 * value that did not survive being written to disk and read back, and a
 * capability the service checks before the type assertion.
 *
 * The second one has a shape here no earlier family had. This driver declares
 * a capability the shared vocabulary did not have - CapShards - and the
 * service checks it before asserting the connection is a Kinesis one, so a
 * capability dropped from the driver produces a message naming the capability
 * rather than a Go type nobody has heard of.
 */

const (
	liveKinesisEndpoint  = "http://127.0.0.1:4567"
	liveKinesisRegion    = "eu-west-1"
	liveKinesisAccessKey = "test"
	liveKinesisSecretKey = "test"
)

// The streams scripts/e2e-kinesis-seed.sh creates.
const (
	liveKinesisOrders   = "MQS-SEED-orders"
	liveKinesisSplit    = "MQS-SEED-split"
	liveKinesisEmpty    = "MQS-SEED-empty"
	liveKinesisOnDemand = "MQS-SEED-ondemand"
)

func requireLiveKinesis(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.Kinesis,
		Name:   "the kinesis e2e environment",
		Start:  "npm run e2e:kinesis:up",
		Probe:  e2e.HTTPGet(liveKinesisEndpoint + "/_localstack/health"),
	})
}

// kinesisStack is the connection service, the Kinesis service and the
// canonical services a board reads through, on a config directory of its own.
type kinesisStack struct {
	connections   *connection.Service
	kinesis       *kinesisservice.Service
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

func newKinesisStack(t *testing.T) *kinesisStack {
	t.Helper()
	if _, ok := driver.Lookup(model.KindKinesis); !ok {
		driver.Register(kinesisdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	conns := newConnSource(registry)
	return &kinesisStack{
		connections: connection.New(
			paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints()),
		kinesis:       kinesisservice.New(conns, settingsService),
		destinations:  destination.New(conns, settingsService),
		subscriptions: subscription.New(conns, settingsService),
		messages:      message.New(conns, settingsService),
		conns:         conns,
		dataFile:      paths.ConnectionsFile,
		settings:      settingsService,
	}
}

// liveKinesisProfile is the environment as a user would configure it: a
// region, a credential, and no address anywhere on the form.
func liveKinesisProfile(name string) model.ConnectionProfile {
	profile := model.ConnectionProfile{
		Name:       name,
		Kind:       model.KindKinesis,
		TimeoutSec: 20,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			kinesisdriver.OptionRegion:      liveKinesisRegion,
			kinesisdriver.OptionEndpointURL: liveKinesisEndpoint,
		},
	}
	profile.SetSecret(kinesisdriver.SecretAccessKeyID, liveKinesisAccessKey)
	profile.SetSecret(kinesisdriver.SecretSecretAccessKey, liveKinesisSecretKey)
	return profile
}

// dial stores a profile and opens it, returning the id a page would hold.
func (s *kinesisStack) dial(t *testing.T, profile model.ConnectionProfile) int {
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

func kinesisContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// testKinesisStreamVia creates a stream through the service layer and removes
// it when the test ends, so nothing a run leaves behind changes the next one.
func testKinesisStreamVia(
	t *testing.T, stack *kinesisStack, connID int, spec kinesisdriver.StreamSpec,
) {
	t.Helper()
	if err := stack.kinesis.CreateStream(kinesisContext(t), connID, spec); err != nil {
		t.Fatalf("creating %s: %v", spec.Name, err)
	}
	t.Cleanup(func() {
		_ = stack.kinesis.RemoveStream(context.Background(), connID, spec.Name)
	})
}

func kinesisTestName(t *testing.T, suffix string) string {
	t.Helper()
	return "MQS-TEST-app-" + strconv.FormatInt(time.Now().UnixNano()%1e9, 36) + suffix
}

/*
 * The whole path in one go: store a profile with no address, dial it, declare
 * a stream, register a consumer, send, and read all three back through the id
 * a page would pass.
 */
func TestLiveKinesisStackRoundTrip(t *testing.T) {
	requireLiveKinesis(t)
	stack := newKinesisStack(t)
	connID := stack.dial(t, liveKinesisProfile("kinesis stack"))
	name := kinesisTestName(t, "-stream")

	testKinesisStreamVia(t, stack, connID, kinesisdriver.StreamSpec{
		Name: name, Shards: 2, RetentionHours: 24,
	})
	if err := stack.kinesis.RegisterConsumer(kinesisContext(t), connID, name, "reader"); err != nil {
		t.Fatalf("register consumer: %v", err)
	}

	if _, err := stack.kinesis.Publish(kinesisContext(t), connID, kinesisdriver.PublishRequest{
		Stream: name, Body: "through the stack", PartitionKey: "stack", Count: 4,
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// The streams board.
	streams, err := stack.destinations.List(kinesisContext(t), connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var listed *model.Destination
	for _, entry := range streams {
		if entry.Ref.Name == name {
			listed = entry
		}
	}
	if listed == nil {
		t.Fatalf("%s is not in the listing the board reads", name)
	}
	if listed.Partitions != 2 {
		t.Errorf("the board would show %d open shards, want 2", listed.Partitions)
	}

	// The shards board, which no canonical service answers - it is the port
	// this family added, reached through the service beside them.
	shards, err := stack.kinesis.Shards(kinesisContext(t), connID, name)
	if err != nil {
		t.Fatalf("Shards: %v", err)
	}
	if len(shards) != 2 {
		t.Errorf("the shards board would show %d rows, want 2", len(shards))
	}

	// The consumers board.
	consumers, err := stack.subscriptions.List(kinesisContext(t), connID)
	if err != nil {
		t.Fatalf("List subscriptions: %v", err)
	}
	var registered *model.Subscription
	for _, entry := range consumers {
		if entry.Ref.Namespace == name && entry.Ref.Name == "reader" {
			registered = entry
		}
	}
	if registered == nil {
		t.Fatalf("the consumer registered on %s is not in the listing the board reads", name)
	}
	if registered.Backlog != model.UnknownMetric {
		t.Errorf("the board would show a backlog of %d, and nothing reports one",
			registered.Backlog)
	}

	// The records board.
	records, err := stack.messages.Query(kinesisContext(t), connID, model.MessageQueryParams{
		Topic: name, MaxResults: 20,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(records) != 4 {
		t.Errorf("the records board would show %d rows, want the 4 that were sent", len(records))
	}
}

/*
 * The profile survives disk, which is where a hosted family's round trip is
 * most easily broken: there is no address to notice missing, and a region or a
 * credential that did not come back produces a connection that saves, reloads
 * and cannot sign.
 */
func TestLiveKinesisProfileSurvivesARestart(t *testing.T) {
	requireLiveKinesis(t)
	stack := newKinesisStack(t)

	profile := liveKinesisProfile("kinesis restart")
	profile.Options[kinesisdriver.OptionStreamPrefix] = "MQS-SEED-"
	created, err := stack.connections.AddConnection(profile)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}

	// A second service on the same file, which is what a restart is, and then
	// a dial through it - not through the one that wrote the profile.
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)
	reopened := connection.New(
		stack.dataFile, stack.settings, newRegistryRuntime(registry), newDescriptorEndpoints())
	if err := reopened.Connect(created.ID); err != nil {
		t.Fatalf("connect after a restart: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Disconnect(created.ID) })

	destinations := destination.New(newConnSource(registry), stack.settings)
	streams, err := destinations.List(
		kinesisContext(t), created.ID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("List after a restart: %v", err)
	}
	if len(streams) == 0 {
		e2e.Missing(t, "the region holds no seeded streams; run `npm run e2e:kinesis:seed`")
	}
	// The prefix came back too, which is the option a listing would silently
	// ignore rather than fail on.
	for _, entry := range streams {
		if !strings.HasPrefix(entry.Ref.Name, "MQS-SEED-") {
			t.Errorf("the reloaded profile's stream prefix was lost: %q got through",
				entry.Ref.Name)
		}
	}
}

/*
 * The capability check runs before the type assertion, and this family is
 * where that matters most: the shards service is the only caller of a
 * capability the shared vocabulary did not have until this driver, so a
 * connection of another family reaching it has to be refused by name.
 */
func TestLiveKinesisServiceRefusesAnotherFamilyByCapability(t *testing.T) {
	requireLiveKinesis(t)
	stack := newKinesisStack(t)
	connID := stack.dial(t, liveKinesisProfile("kinesis capability"))

	// The connection is this family's, so every call resolves - which is what
	// makes the negative below about the capability rather than about the id.
	if _, err := stack.kinesis.Shards(kinesisContext(t), connID, liveKinesisOrders); err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:kinesis:seed` (%v)",
			liveKinesisOrders, err)
	}

	if _, err := stack.kinesis.Shards(kinesisContext(t), 9999, "anything"); err == nil {
		t.Error("answered for a connection id that was never opened")
	}
}

/*
 * The sidebar's own question, asked the way the bridge asks it.
 *
 * Nothing else in this suite reaches Capabilities() through a connection id,
 * and it is what decides which pages are drawn - so a driver that stopped
 * declaring the shard capability would take a finished page out of the
 * sidebar with every other test still green.
 */
func TestLiveKinesisCapabilitiesReachTheSidebar(t *testing.T) {
	requireLiveKinesis(t)
	stack := newKinesisStack(t)
	connID := stack.dial(t, liveKinesisProfile("kinesis capabilities"))

	conn, err := stack.conns(connID)
	if err != nil {
		t.Fatalf("resolve the connection: %v", err)
	}
	declared := conn.Capabilities()

	for _, capability := range []model.Capability{
		model.CapDestinationList,
		model.CapShards,
		model.CapMessageQuery,
		model.CapPublish,
		model.CapSubscriptionList,
	} {
		if !declared.Has(capability) {
			t.Errorf("a live connection does not declare %s, and a page gates on it", capability)
		}
	}

	// The backlog, degraded rather than absent: the consumers page is drawn
	// and the column explains itself.
	if declared.Has(model.CapSubscriptionLag) {
		t.Error("a live connection declares a subscription backlog, and nothing reports one")
	}
	if reason, degraded := declared.DegradedReason(model.CapSubscriptionLag); !degraded {
		t.Error("the backlog is absent with no reason a page could show")
	} else if !strings.HasPrefix(reason, "mq.kinesis.degraded.") {
		t.Errorf("degraded reason %q is not this driver's i18n key", reason)
	}

	// And the caveat, which is the one that must not be SQS's.
	caveat, warned := declared.Caveat(model.CapMessageQuery)
	if !warned {
		t.Fatal("browsing carries no caveat, and it does spend a shard's read budget")
	}
	if !strings.HasPrefix(caveat, "mq.kinesis.caveat.") {
		t.Errorf("caveat %q is not this driver's i18n key", caveat)
	}
}
