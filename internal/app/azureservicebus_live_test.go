package app

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	servicebusdriver "github.com/amigoer/mq-studio/internal/driver/azureservicebus"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	servicebusservice "github.com/amigoer/mq-studio/internal/service/azureservicebus"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/destination"
	"github.com/amigoer/mq-studio/internal/service/message"
	"github.com/amigoer/mq-studio/internal/service/routing"
	"github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/service/subscription"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

/*
 * The Azure Service Bus stack from the outside, through a connection id.
 *
 * The driver's own live tests hold a *Conn. These hold nothing: they store a
 * profile, dial it, and then ask the service layer the way a page does - with
 * an integer. What that covers and the driver's tests do not is the chain in
 * between, where the two failures this project has actually had both lived: a
 * value that did not survive being written to disk and read back, and a
 * capability the service checks before the type assertion.
 *
 * This family's round trip is one field wider than either hosted family before
 * it, because it is the first of the three with an address: a namespace, a
 * policy name, two secrets of different shapes, a name prefix and an emulator
 * management host all have to come back off disk intact, and getting any of
 * them wrong produces a profile that saves, reloads and then cannot dial.
 */

const (
	liveServiceBusNamespace  = "localhost:5672"
	liveServiceBusManagement = "127.0.0.1:5300"
	liveServiceBusKeyName    = "RootManageSharedAccessKey"
	liveServiceBusKey        = "SAS_KEY_VALUE"
)

// The entities tests/e2e/azure-servicebus/config.json declares and
// scripts/e2e-azure-servicebus-seed.sh fills.
const (
	liveServiceBusOrders   = "mqs-seed-orders"
	liveServiceBusFailures = "mqs-seed-failures"
	liveServiceBusQuiet    = "mqs-seed-quiet"
	liveServiceBusEvents   = "mqs-seed-events"
	liveServiceBusOrphaned = "mqs-seed-orphaned"

	liveServiceBusSubAll    = "mqs-seed-events-all"
	liveServiceBusSubRed    = "mqs-seed-events-red"
	liveServiceBusSubOrders = "mqs-seed-events-orders"
)

func requireLiveServiceBus(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.AzureServiceBus,
		Name:   "the azure service bus e2e environment",
		Start:  "npm run e2e:azure-servicebus:up",
		Probe:  e2e.HTTPGet("http://" + liveServiceBusManagement + "/health"),
	})
}

// serviceBusStack is the connection service, the Service Bus service and the
// canonical services a board reads through, on a config directory of its own.
type serviceBusStack struct {
	connections   *connection.Service
	servicebus    *servicebusservice.Service
	destinations  *destination.Service
	subscriptions *subscription.Service
	messages      *message.Service
	routing       *routing.Service
	// conns is what the bridge holds to answer capability questions without
	// going through a domain service, which is how the sidebar decides what to
	// draw and why.
	conns func(connID int) (driver.Conn, error)
	// dataFile is where the profiles land, so a second service can be opened
	// on the same store and prove what actually survived disk.
	dataFile string
	settings *settings.Service
}

func newServiceBusStack(t *testing.T) *serviceBusStack {
	t.Helper()
	if _, ok := driver.Lookup(model.KindAzureServiceBus); !ok {
		driver.Register(servicebusdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	conns := newConnSource(registry)
	return &serviceBusStack{
		connections: connection.New(
			paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints()),
		servicebus:    servicebusservice.New(conns, settingsService),
		destinations:  destination.New(conns, settingsService),
		subscriptions: subscription.New(conns, settingsService),
		messages:      message.New(conns, settingsService),
		routing:       routing.New(conns, settingsService),
		conns:         conns,
		dataFile:      paths.ConnectionsFile,
		settings:      settingsService,
	}
}

// liveServiceBusProfile is the environment as a user would configure it.
func liveServiceBusProfile(name string) model.ConnectionProfile {
	profile := model.ConnectionProfile{
		Name:       name,
		Kind:       model.KindAzureServiceBus,
		Endpoints:  liveServiceBusNamespace,
		TimeoutSec: 20,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			servicebusdriver.OptionSharedAccessKeyName: liveServiceBusKeyName,
			servicebusdriver.OptionEmulatorManagement:  liveServiceBusManagement,
		},
	}
	profile.SetSecret(servicebusdriver.SecretSharedAccessKey, liveServiceBusKey)
	return profile
}

// dial stores a profile and opens it, returning the id a page would hold.
func (s *serviceBusStack) dial(t *testing.T, profile model.ConnectionProfile) int {
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

func serviceBusContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// testServiceBusEntityVia creates an entity through the service layer and
// removes it when the test ends, so nothing a run leaves behind changes the
// next one.
func testServiceBusEntityVia(
	t *testing.T, stack *serviceBusStack, connID int, spec servicebusdriver.EntitySpec,
) {
	t.Helper()
	if err := stack.servicebus.CreateEntity(serviceBusContext(t), connID, spec); err != nil {
		t.Fatalf("creating %s: %v", spec.Name, err)
	}
	t.Cleanup(func() {
		_ = stack.servicebus.RemoveEntity(context.Background(), connID, spec.Name)
	})
}

func serviceBusTestName(t *testing.T, suffix string) string {
	t.Helper()
	return "mqs-test-app-" + strconv.FormatInt(time.Now().UnixNano()%1e9, 36) + suffix
}

/*
 * The whole path in one go: store a profile with an address, dial it, declare
 * a topic and a subscription, send, and read all three back through the id a
 * page would pass.
 */
func TestLiveServiceBusStackRoundTrip(t *testing.T) {
	requireLiveServiceBus(t)
	stack := newServiceBusStack(t)
	connID := stack.dial(t, liveServiceBusProfile("service bus stack"))
	topic := serviceBusTestName(t, "-topic")
	reader := serviceBusTestName(t, "-reader")

	testServiceBusEntityVia(t, stack, connID, servicebusdriver.EntitySpec{
		Name: topic, Kind: servicebusdriver.EntityTopic,
	})
	if err := stack.servicebus.CreateSubscription(serviceBusContext(t), connID,
		servicebusdriver.SubscriptionSpec{
			Topic: topic, Name: reader, LockDurationSec: 60,
		}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	result, err := stack.servicebus.Send(serviceBusContext(t), connID, servicebusdriver.SendRequest{
		Entity: topic, Body: "through the stack", Count: 4, Subject: "order",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.Sent != 4 {
		t.Fatalf("sent %d, want 4", result.Sent)
	}

	destinations, err := stack.destinations.List(serviceBusContext(t), connID, model.DestinationFilter{})
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
	if listed.Subscribers != 1 {
		t.Errorf("subscribers = %d, want the one subscription created", listed.Subscribers)
	}
	// A topic holds nothing, on every endpoint there is.
	if listed.Depth != model.UnknownMetric {
		t.Errorf("depth = %d; a topic holds nothing countable", listed.Depth)
	}

	groups, err := stack.subscriptions.List(serviceBusContext(t), connID)
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
	if found.Ref.Namespace != topic {
		t.Errorf("the subscription belongs to %q, want %q", found.Ref.Namespace, topic)
	}

	// The browse takes a queue or a topic-and-subscription, which is where
	// this family's vocabulary crosses the canonical service: the filter map
	// carries the subscription because the canonical params have no field for
	// one.
	held, err := stack.messages.Query(serviceBusContext(t), connID, model.MessageQueryParams{
		Topic:      topic,
		MaxResults: 20,
		Filters:    map[string]string{servicebusdriver.FilterSubscription: reader},
	})
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if len(held) != 4 {
		t.Errorf("the browse found %d of the 4 sent", len(held))
	}
	for _, item := range held {
		if item.Tags != "order" {
			t.Errorf("a message came back with subject %q", item.Tags)
		}
	}
}

/*
 * What actually survived being written to disk.
 *
 * A second connection service on the same file is what a restart is, and it is
 * the only way to prove a profile is dialable after one. This family has more
 * to lose than either hosted family before it: an address, a policy name, a
 * key, a prefix and an emulator host.
 */
func TestLiveServiceBusProfileSurvivesARestart(t *testing.T) {
	requireLiveServiceBus(t)
	stack := newServiceBusStack(t)

	profile := liveServiceBusProfile("service bus across a restart")
	profile.Options[servicebusdriver.OptionEntityPrefix] = "mqs-seed-"
	created, err := stack.connections.AddConnection(profile)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}

	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)
	reopened := connection.New(
		stack.dataFile, stack.settings, newRegistryRuntime(registry), newDescriptorEndpoints())

	if err := reopened.Connect(created.ID); err != nil {
		t.Fatalf("connect after a restart: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Disconnect(created.ID) })

	// And the prefix came back with it, which is the option a listing reads.
	destinations := destination.New(newConnSource(registry), stack.settings)
	listed, err := destinations.List(serviceBusContext(t), created.ID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	if len(listed) == 0 {
		e2e.Missing(t, "the namespace holds nothing; run npm run e2e:azure-servicebus:seed")
	}
	for _, entry := range listed {
		if !strings.HasPrefix(entry.Ref.Name, "mqs-seed-") {
			t.Errorf("%s is outside the prefix the profile was saved with", entry.Ref.Name)
		}
	}
}

/*
 * The capabilities a page gates on, read the way the bridge reads them.
 *
 * The sidebar asks the connection rather than the driver package, so this is
 * the layer where a capability that never reached the registry would show. It
 * also pins the narrowing the emulator causes: everything except the backlog
 * is supported, and the backlog is degraded with a reason rather than absent.
 */
func TestLiveServiceBusCapabilitiesReachTheBridge(t *testing.T) {
	requireLiveServiceBus(t)
	stack := newServiceBusStack(t)
	connID := stack.dial(t, liveServiceBusProfile("service bus capabilities"))

	conn, err := stack.conns(connID)
	if err != nil {
		t.Fatalf("resolve the connection: %v", err)
	}
	declared := conn.Capabilities()

	for _, capability := range []model.Capability{
		model.CapDestinationList,
		model.CapDestinationCreate,
		model.CapDestinationDelete,
		model.CapSubscriptionList,
		model.CapMessageQuery,
		model.CapPublish,
		model.CapDLQ,
		model.CapRouting,
		model.CapRoutingAdmin,
	} {
		if !declared.Has(capability) {
			t.Errorf("%s did not reach the bridge", capability)
		}
	}

	// The one thing that varies by endpoint, and the emulator is the endpoint
	// that cannot answer it.
	if declared.Has(model.CapSubscriptionLag) {
		t.Error("the backlog is offered against an emulator that reports no counts")
	}
	if _, degraded := declared.DegradedReason(model.CapSubscriptionLag); !degraded {
		t.Error("the backlog is neither supported nor explained")
	}

	// And no caveat anywhere, which is this family's distinguishing fact: a
	// peek takes nothing, so the messages page has nothing to warn about.
	if caveat, warned := declared.Caveat(model.CapMessageQuery); warned {
		t.Errorf("browsing warns %q, and a peek takes nothing", caveat)
	}
}

/*
 * A capability the connection does not declare is refused by the service
 * before the type assertion, and the message names the capability.
 *
 * Worth its own test because the failure it prevents is silent in the other
 * direction: a service that asserted the type first would answer a page with a
 * Go type name it has never heard of.
 */
func TestLiveServiceBusRefusesWhatItDoesNotDeclare(t *testing.T) {
	requireLiveServiceBus(t)
	stack := newServiceBusStack(t)
	connID := stack.dial(t, liveServiceBusProfile("service bus refusals"))

	// Resetting a subscription's position: this family has none, so the
	// canonical service must refuse rather than reach the driver.
	err := stack.subscriptions.ResetOffset(serviceBusContext(t), connID, model.ResetOffsetRequest{
		Group: liveServiceBusSubAll, Timestamp: time.Now().UnixMilli(),
	})
	if err == nil {
		t.Fatal("reset a position on a family that has none")
	}
	if !strings.Contains(err.Error(), string(model.CapOffsetReset)) {
		t.Errorf("the refusal does not name the capability: %v", err)
	}
}

/*
 * The routing page through the canonical service, which is the only other
 * family this service has ever answered for.
 *
 * Worth reading through the service layer rather than the driver because the
 * routing service is RabbitMQ's, and a Service Bus connection reaching it has
 * to come back with rules in the shapes that page draws.
 */
func TestLiveServiceBusRulesReachTheRoutingService(t *testing.T) {
	requireLiveServiceBus(t)
	stack := newServiceBusStack(t)
	connID := stack.dial(t, liveServiceBusProfile("service bus routing"))

	exchanges, err := stack.routing.Exchanges(serviceBusContext(t), connID, "")
	if err != nil {
		t.Fatalf("exchanges: %v", err)
	}
	found := false
	for _, exchange := range exchanges {
		if exchange.Ref.Name == liveServiceBusEvents {
			found = true
		}
		// The routing page only ever shows topics: a queue keeps what is sent
		// to it, which is what an exchange never does.
		if exchange.Attributes[servicebusdriver.AttrEntityType] != servicebusdriver.EntityTopic {
			t.Errorf("%s is on the routing page and is not a topic", exchange.Ref.Name)
		}
	}
	if !found {
		e2e.Missing(t, "%s is not a routing point; run npm run e2e:azure-servicebus:seed",
			liveServiceBusEvents)
	}

	bindings, err := stack.routing.Bindings(serviceBusContext(t), connID, "")
	if err != nil {
		t.Fatalf("bindings: %v", err)
	}
	rules := map[string]*model.Binding{}
	for _, binding := range bindings {
		rules[binding.Destination+"/"+binding.PropertiesKey] = binding
		// The service numbers the rows for the renderer, which is a thing the
		// driver does not do and the page needs.
		if binding.ID == 0 {
			t.Errorf("a rule on %s/%s reached the page with no list key",
				binding.Source, binding.Destination)
		}
	}
	for _, want := range []string{
		liveServiceBusSubAll + "/$Default",
		liveServiceBusSubRed + "/red-only",
		liveServiceBusSubOrders + "/orders-only",
	} {
		if rules[want] == nil {
			e2e.Missing(t, "%s is missing; run npm run e2e:azure-servicebus:seed", want)
		}
	}
}

/*
 * The dead letters through the canonical message service.
 *
 * The service takes RocketMQ's word for it - a "group" - and this family puts
 * an entity path there, because a dead letter belongs to the entity rather
 * than to whoever was reading it. Reading it through the service is what
 * proves the two agree.
 */
func TestLiveServiceBusDeadLettersReachTheMessageService(t *testing.T) {
	requireLiveServiceBus(t)
	stack := newServiceBusStack(t)
	connID := stack.dial(t, liveServiceBusProfile("service bus dead letters"))

	dead, err := stack.messages.DLQ(serviceBusContext(t), connID, liveServiceBusFailures, 50)
	if err != nil {
		t.Fatalf("dlq: %v", err)
	}
	if len(dead) != 4 {
		e2e.Missing(t, "%s holds %d dead letters, seeded 4; run npm run e2e:azure-servicebus:seed",
			liveServiceBusFailures, len(dead))
	}
	for _, item := range dead {
		if item.Status != model.MsgDLQ {
			t.Errorf("a dead letter reached the page as %q", item.Status)
		}
	}

	// The retry backlog is not a thing here, and the service has to say so
	// rather than answering with the ordinary one.
	if _, err := stack.messages.Retry(
		serviceBusContext(t), connID, liveServiceBusFailures, 10); err == nil {
		t.Error("a retry backlog came back, and Service Bus keeps none")
	}
}
