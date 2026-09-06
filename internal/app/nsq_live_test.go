package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	nsqdriver "github.com/amigoer/mq-studio/internal/driver/nsq"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/cluster"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/destination"
	nsqservice "github.com/amigoer/mq-studio/internal/service/nsq"
	"github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/service/subscription"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

/*
 * The NSQ stack from the outside, through a connection id.
 *
 * The driver's own live tests hold a *Conn. These hold nothing: they store a
 * profile, dial it, and then ask the service layer the way a page does - with
 * an integer. What that covers and the driver's tests do not is the chain in
 * between, where the two failures this project has actually had both lived: a
 * value that did not survive being written to disk and read back, and a
 * capability the service checks before the type assertion.
 *
 * This family has no credentials at all, which moves the first risk rather
 * than removing it. What has to survive the round trip here is the nsqlookupd
 * list, and losing it is nearly invisible: every board still fills, every
 * figure is still right, and only the directory half of the cluster page
 * quietly stops being offered.
 */

const (
	liveNSQD1    = "http://127.0.0.1:4151"
	liveNSQD2    = "http://127.0.0.1:4153"
	liveLookupd1 = "http://127.0.0.1:4161"
	liveLookupd2 = "http://127.0.0.1:4163"
)

func requireLiveNSQ(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.NSQ,
		Name:   "the nsq e2e cluster",
		Start:  "npm run e2e:nsq:up",
		Probe:  e2e.HTTPGet(liveNSQD1 + "/ping"),
	})
}

// nsqStack is the connection service, the NSQ service and the canonical
// services a board reads through, on a config directory of its own.
type nsqStack struct {
	connections  *connection.Service
	nsq          *nsqservice.Service
	destinations *destination.Service
	consumers    *subscription.Service
	cluster      *cluster.Service
	// conns is what the bridge holds to answer capability questions without
	// going through a domain service, which is how the sidebar decides what to
	// draw and why.
	conns func(connID int) (driver.Conn, error)
}

func newNSQStack(t *testing.T) *nsqStack {
	t.Helper()
	if _, ok := driver.Lookup(model.KindNSQ); !ok {
		driver.Register(nsqdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	conns := newConnSource(registry)
	return &nsqStack{
		connections: connection.New(
			paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints()),
		nsq:          nsqservice.New(conns, settingsService),
		destinations: destination.New(conns, settingsService),
		consumers:    subscription.New(conns, settingsService),
		cluster:      cluster.New(paths.TPSHistoryFile, conns, settingsService),
		conns:        conns,
	}
}

// liveNSQProfile is the cluster as a user would configure it, with or without
// the optional discovery tier.
func liveNSQProfile(name string, withLookupd bool) model.ConnectionProfile {
	profile := model.ConnectionProfile{
		Name:       name,
		Kind:       model.KindNSQ,
		Endpoints:  liveNSQD1 + "," + liveNSQD2,
		TimeoutSec: 10,
		Options:    map[string]string{},
	}
	if withLookupd {
		profile.Options[nsqdriver.OptionLookupd] = liveLookupd1 + "," + liveLookupd2
	}
	return profile
}

// dial stores a profile and opens it, returning the id a page would hold.
func (s *nsqStack) dial(t *testing.T, profile model.ConnectionProfile) int {
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

func nsqContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// testTopicVia creates a topic through the service layer and removes it when
// the test ends, so nothing a run leaves behind changes the next one.
func testTopicVia(t *testing.T, stack *nsqStack, connID int, name string) {
	t.Helper()
	if err := stack.nsq.CreateTopic(nsqContext(t), connID, name); err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = stack.nsq.RemoveTopic(context.Background(), connID, name)
	})
}

/*
 * The whole path in one go: store a profile, dial it, then declare a topic and
 * a channel, publish to it, and read both back through the id a page would
 * pass.
 */
func TestLiveNSQStackRoundTrip(t *testing.T) {
	requireLiveNSQ(t)
	stack := newNSQStack(t)
	connID := stack.dial(t, liveNSQProfile("nsq stack", true))
	const topic = "MQS.TEST.stack"

	testTopicVia(t, stack, connID, topic)
	if err := stack.nsq.CreateChannel(nsqContext(t), connID, topic, "analytics"); err != nil {
		t.Fatalf("create channel: %v", err)
	}

	result, err := stack.nsq.Publish(nsqContext(t), connID, nsqdriver.PublishRequest{
		Topic: topic, Body: "through the stack", Count: 4,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.Sent != 4 {
		t.Fatalf("sent %d, want 4", result.Sent)
	}

	destinations, err := stack.destinations.List(nsqContext(t), connID, model.DestinationFilter{})
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
	if listed.Depth != 4 {
		t.Errorf("depth = %d, want the 4 published", listed.Depth)
	}

	subscriptions, err := stack.consumers.List(nsqContext(t), connID)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	var channel *model.Subscription
	for _, entry := range subscriptions {
		if entry.Ref.Namespace == topic && entry.Ref.Name == "analytics" {
			channel = entry
		}
	}
	if channel == nil {
		t.Fatal("the channel the stack created is not in the listing")
	}
	if channel.Backlog != 4 {
		t.Errorf("backlog = %d, want the 4 published", channel.Backlog)
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
 * The nsqlookupd list has to survive being written to disk and read back.
 *
 * It is the only thing on this family's connection form that can be lost
 * silently. Every board still fills without it and every figure is still
 * right; what goes is the directory half of the cluster page, which stops
 * being offered rather than reporting an error - so nothing but this would
 * notice.
 */
func TestLiveNSQDirectorySurvivesDisk(t *testing.T) {
	requireLiveNSQ(t)
	stack := newNSQStack(t)
	connID := stack.dial(t, liveNSQProfile("nsq with a directory", true))

	directory, err := stack.cluster.DirectoryNodes(nsqContext(t), connID)
	if err != nil {
		t.Fatalf("directory nodes: %v", err)
	}
	if len(directory) != 2 {
		t.Fatalf("listed %d nsqlookupd, want the 2 the profile named", len(directory))
	}

	// A second stack on the same store would be the stronger test, but the
	// store is per temp directory; what this asserts instead is that the
	// profile came back off disk with the option intact.
	stored, err := stack.connections.GetConnection(connID)
	if err != nil {
		t.Fatalf("read the stored profile: %v", err)
	}
	if !strings.Contains(stored.Options[nsqdriver.OptionLookupd], liveLookupd2) {
		t.Errorf("the stored profile names %q, which has lost an nsqlookupd",
			stored.Options[nsqdriver.OptionLookupd])
	}
}

/*
 * A connection with no discovery tier keeps every other page and says why that
 * one is missing.
 *
 * The listing comes back empty rather than failing, because a family with no
 * tier at all is a fact rather than an error - so the empty list on its own
 * cannot tell "there is no such thing here" from "you did not configure one".
 * What separates them is the degraded reason on the capability, which is what
 * the sidebar shows and what this asserts is there.
 */
func TestLiveNSQWithoutADirectoryDegradesRatherThanFails(t *testing.T) {
	requireLiveNSQ(t)
	stack := newNSQStack(t)
	connID := stack.dial(t, liveNSQProfile("nsq without a directory", false))

	overview, nodes, err := stack.cluster.Overview(nsqContext(t), connID)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(nodes) != 2 || overview.TotalNodes != 2 {
		t.Errorf("nodes = %d of %d, want both daemons", len(nodes), overview.TotalNodes)
	}

	directory, err := stack.cluster.DirectoryNodes(nsqContext(t), connID)
	if err != nil {
		t.Fatalf("directory nodes: %v", err)
	}
	if len(directory) != 0 {
		t.Errorf("listed %d nsqlookupd on a connection that names none", len(directory))
	}

	conn, err := stack.conns(connID)
	if err != nil {
		t.Fatalf("resolve the connection: %v", err)
	}
	if conn.Capabilities().Has(model.CapDirectory) {
		t.Error("a connection naming no nsqlookupd claims a discovery tier")
	}
	reason, degraded := conn.Capabilities().DegradedReason(model.CapDirectory)
	if !degraded {
		t.Fatal("the missing discovery tier is neither supported nor explained")
	}
	if !strings.HasPrefix(reason, "mq.nsq.") {
		t.Errorf("reason = %q, which is not an i18n key the sidebar can resolve", reason)
	}
}
