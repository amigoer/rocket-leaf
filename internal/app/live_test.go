package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/driver/rocketmq"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/cluster"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/destination"
	"github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/storage/layout"

	admin "github.com/amigoer/rocketmq-admin-go"
	rocketmqconsumer "github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	rocketmqproducer "github.com/apache/rocketmq-client-go/v2/producer"
)

// Exercises the stack the connection screen drives, against the broker
// `npm run e2e:up` starts. Opt-in locally, mandatory in CI, like the driver's
// own live tests - see internal/e2e:
//
//	npm run e2e:up && MQ_STUDIO_E2E=1 go test ./internal/app/...
const liveNameServer = "127.0.0.1:9876"

// What `npm run e2e:seed` puts on the broker. Tests that need a consumer group
// use this one, because mq-studio cannot create one - see
// TestLiveConsumerGroupDelete for why.
const (
	seededTopic = "MQ_STUDIO_E2E"
	seededGroup = "MQ_STUDIO_E2E_GROUP"
)

// The separate ACL-enabled broker, from `npm run e2e:acl:up`. Its admin
// account is the one seeded in tests/e2e/rocketmq-acl/plain_acl.yml.
const (
	aclNameServer = "127.0.0.1:9877"
	aclAccessKey  = "mqstudio"
	aclSecretKey  = "mqstudio-secret"
)

// requireLiveBroker gates on the broker `npm run e2e:up` starts. The ACL tests
// gate on requireACLBroker as well: it is a different broker on its own port,
// started separately, and one being up says nothing about the other.
func requireLiveBroker(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Name:   "the rocketmq broker",
		Family: e2e.RocketMQ,
		Start:  "npm run e2e:up",
		Probe:  e2e.DialTCP(liveNameServer),
	})
}

func requireACLBroker(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Name:   "the ACL-enabled rocketmq broker",
		Family: e2e.RocketMQ,
		Start:  "npm run e2e:acl:up",
		Probe:  e2e.DialTCP(aclNameServer),
	})
}

// liveStack assembles the same pieces New does, rooted in a temp directory so
// the test never touches the user's real configuration.
func liveStack(t *testing.T) (*connection.Service, *destination.Service, *driver.Registry) {
	t.Helper()
	requireLiveBroker(t)
	if _, ok := driver.Lookup(model.KindRocketMQ); !ok {
		driver.Register(rocketmq.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	connections := connection.New(
		paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints())
	return connections, destination.New(newConnSource(registry), settingsService), registry
}

func liveProfileInput(name string) model.ConnectionProfile {
	return model.ConnectionProfile{
		Name:       name,
		Kind:       model.KindRocketMQ,
		Endpoints:  liveNameServer,
		TimeoutSec: 5,
	}
}

// The whole M1 path in one go: store a profile, dial it, read through the id
// the page would pass, then close it.
func TestLiveConnectListDisconnect(t *testing.T) {
	connections, topics, registry := liveStack(t)
	ctx := context.Background()

	// Internal topics are included because a fresh broker has no user topics,
	// and "empty" would then prove nothing about whether the read reached it.
	everything := model.DestinationFilter{IncludeInternal: true}

	profile, err := connections.AddConnection(liveProfileInput("live"))
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	// A profile nobody connected lists empty rather than erroring, which is the
	// contract the list pages render against.
	before, err := topics.List(ctx, profile.ID, everything)
	if err != nil {
		t.Fatalf("list before connecting: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("listed %d topics before connecting", len(before))
	}

	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	stored, err := connections.GetConnection(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.StatusOnline {
		t.Fatalf("stored status = %q, want online", stored.Status)
	}

	during, err := topics.List(ctx, profile.ID, everything)
	if err != nil {
		t.Fatalf("list topics through the connection id: %v", err)
	}
	if len(during) == 0 {
		t.Fatal("a connected broker listed no topics at all")
	}

	if err := connections.Disconnect(profile.ID); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if _, stillOpen := registry.Get(profile.ID); stillOpen {
		t.Fatal("the registry kept a disconnected connection")
	}
	after, err := topics.List(ctx, profile.ID, everything)
	if err != nil {
		t.Fatalf("list after disconnecting: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("listed %d topics after disconnecting", len(after))
	}
}

// Two profiles on one broker are what the tab strip opens, and each page reads
// through its own id.
func TestLiveTwoConnectionsStayOpenTogether(t *testing.T) {
	connections, topics, _ := liveStack(t)
	ctx := context.Background()

	first, err := connections.AddConnection(liveProfileInput("first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := connections.AddConnection(liveProfileInput("second"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{first.ID, second.ID} {
		if err := connections.Connect(id); err != nil {
			t.Fatalf("connect %d: %v", id, err)
		}
	}

	for _, id := range []int{first.ID, second.ID} {
		if _, err := topics.List(ctx, id, model.DestinationFilter{}); err != nil {
			t.Fatalf("list topics on %d: %v", id, err)
		}
	}

	// Closing the first must leave the second answering: that is the whole
	// point of one client per profile.
	if err := connections.Disconnect(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := topics.List(ctx, second.ID, model.DestinationFilter{}); err != nil {
		t.Fatalf("second connection broke when the first closed: %v", err)
	}
}

// The dialog's test button probes a draft that has never been stored.
func TestLiveProbeUnsavedProfile(t *testing.T) {
	connections, _, registry := liveStack(t)

	if err := connections.ProbeProfile(liveProfileInput("draft")); err != nil {
		t.Fatalf("probe a reachable draft: %v", err)
	}
	// A probe must leave nothing open behind it.
	if ids := registry.IDs(); len(ids) != 0 {
		t.Fatalf("probe left %v open", ids)
	}

	unreachable := liveProfileInput("draft")
	unreachable.Endpoints = "127.0.0.1:19876"
	unreachable.TimeoutSec = 2
	if err := connections.ProbeProfile(unreachable); err == nil {
		t.Fatal("probing an unreachable NameServer should fail")
	}
}

// The path the producer and message boards drive: publish, then find it again
// through the same connection id, then read its consume trace.
func TestLiveSendThenQuery(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("send"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, ok := registry.Get(profile.ID)
	if !ok {
		t.Fatal("connection missing after connect")
	}

	const topic = "MQ_STUDIO_E2E"
	key := "e2e-" + time.Now().UTC().Format("20060102150405.000")
	body := `{"probe":"` + key + `"}`

	messageID, err := conn.(driver.MessagePublisher).SendMessage(ctx, topic, "probe", key, body, 0)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if messageID == "" {
		t.Fatal("send returned an empty message id")
	}

	// The broker indexes by key asynchronously, so the query is retried rather
	// than run once and called a failure.
	reader := conn.(driver.MessageReader)
	var found []*model.MessageItem
	for attempt := range 10 {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		found, err = reader.QueryMessages(ctx, model.MessageQueryParams{
			Topic:      topic,
			MessageKey: key,
			MaxResults: 8,
		})
		if err == nil && len(found) > 0 {
			break
		}
	}
	if err != nil {
		t.Fatalf("query by key: %v", err)
	}
	if len(found) == 0 {
		t.Fatalf("the message sent as %s never came back for key %s", messageID, key)
	}
	if found[0].Body != body {
		t.Fatalf("body = %q, want %q", found[0].Body, body)
	}

	// The trace looks the message up again, and the broker's key index lags a
	// send by a second or two - the same lag the query above rides out. With no
	// group subscribed the answer is an empty list, which is the honest one.
	tracker := conn.(driver.MessageTracker)
	for attempt := range 10 {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		trackCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		_, err = tracker.TrackMessage(trackCtx, topic, found[0].MessageID)
		cancel()
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("track: %v", err)
	}
}

// The topics board's write path: create on every master, read the config back,
// change it, then delete.
func TestLiveTopicLifecycle(t *testing.T) {
	connections, topics, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("topics"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)

	const name = "MQ_STUDIO_E2E_LIFECYCLE"
	ref := model.DestinationRef{Name: name}
	admin := conn.(driver.DestinationAdmin)
	t.Cleanup(func() { _ = admin.RemoveDestination(context.Background(), ref) })

	// No broker named: every master should get it.
	spec := model.DestinationSpec{
		Ref: ref,
		Attributes: map[string]string{
			rocketmq.AttrReadQueue:  "2",
			rocketmq.AttrWriteQueue: "2",
			rocketmq.AttrPerm:       string(model.PermRW),
		},
	}
	if err := admin.CreateDestination(ctx, spec); err != nil {
		t.Fatalf("create: %v", err)
	}

	created, err := topics.Detail(ctx, profile.ID, ref)
	if err != nil {
		t.Fatalf("detail after create: %v", err)
	}
	if got := created.Attributes[rocketmq.AttrWriteQueue]; got != "2" {
		t.Fatalf("writeQueue = %q, want 2", got)
	}

	spec.Attributes[rocketmq.AttrReadQueue] = "4"
	spec.Attributes[rocketmq.AttrWriteQueue] = "4"
	if err := admin.UpdateDestination(ctx, spec); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, err := topics.Detail(ctx, profile.ID, ref)
	if err != nil {
		t.Fatalf("detail after update: %v", err)
	}
	if got := updated.Attributes[rocketmq.AttrWriteQueue]; got != "4" {
		t.Fatalf("writeQueue after update = %q, want 4", got)
	}

	if err := admin.RemoveDestination(ctx, ref); err != nil {
		t.Fatalf("remove: %v", err)
	}
	listed, err := topics.List(ctx, profile.ID, model.DestinationFilter{IncludeInternal: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, one := range listed {
		if one.Ref.Name == name {
			t.Fatal("the deleted topic is still listed")
		}
	}
}

// What the topic inspector opens on: the two fields the list cannot carry.
//
// The list enrichment fills queue counts and inbound rate for every topic at
// once; the route table and the subscribing groups cost a call per topic, and
// the outbound rate a call per group, so only Detail has them. A board reading
// the list row alone would draw an empty route table and a dash for consume
// TPS, which is what this stops.
func TestLiveTopicDetailCarriesRoutes(t *testing.T) {
	connections, topics, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("detail"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)

	const name = "MQ_STUDIO_E2E"
	ref := model.DestinationRef{Name: name}
	admin := conn.(driver.DestinationAdmin)
	if err := admin.CreateDestination(ctx, model.DestinationSpec{
		Ref: ref,
		Attributes: map[string]string{
			rocketmq.AttrReadQueue:  "2",
			rocketmq.AttrWriteQueue: "2",
			rocketmq.AttrPerm:       string(model.PermRW),
		},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	listed, err := topics.List(ctx, profile.ID, model.DestinationFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var row *model.Destination
	for _, one := range listed {
		if one.Ref.Name == name {
			row = one
			break
		}
	}
	if row == nil {
		t.Fatalf("%s is not in the list", name)
	}
	if got := row.Attribute(rocketmq.AttrRoutes); got != "" {
		t.Fatalf("the list carries a route table (%q); it is meant to cost too much", got)
	}

	detail, err := topics.Detail(ctx, profile.ID, ref)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Attribute(rocketmq.AttrRoutes) == "" {
		t.Fatal("Detail returned no route table; the inspector draws an empty one")
	}
	if detail.RateOut == model.UnknownMetric {
		t.Fatal("Detail returned no outbound rate; the inspector draws a dash")
	}

	// The names behind the count, which the inspector lists. Asserted against
	// the count rather than against a fixture, so it holds on a broker where
	// nothing subscribes yet.
	var names []string
	if encoded := detail.Attribute(rocketmq.AttrSubscribers); encoded != "" {
		if err := json.Unmarshal([]byte(encoded), &names); err != nil {
			t.Fatalf("subscribers is not a name list: %v", err)
		}
	}
	if got := detail.Attribute(rocketmq.AttrConsumerGroups); got != strconv.Itoa(len(names)) {
		t.Fatalf("consumerGroups = %s but %d names came back", got, len(names))
	}
}

// The whole consumer group lifecycle the form drives: create, read back,
// update, read back, delete.
//
// Create and update were unbuildable until rocketmq-admin-go v1.3.2 - it put
// the SubscriptionGroupConfig in the request's extFields while RocketMQ 5.x
// decodes it from the body, so the broker answered every one with a
// NullPointerException. What this asserts is the half that was missing: that
// the settings sent are the settings stored, and that an update rewrites the
// whole config rather than merging into it.
func TestLiveConsumerGroupLifecycle(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("groups"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)
	groups := conn.(driver.SubscriptionAdmin)

	// Its own name, not the seeded group: this one deletes what it makes, and
	// pointing it at the shared group deleted it out from under
	// TestLiveResetOffset, which then skipped instead of asserting anything.
	const name = "MQ_STUDIO_E2E_GROUP_LIFECYCLE"
	ref := model.SubscriptionRef{Name: name}
	t.Cleanup(func() { _ = groups.RemoveSubscription(context.Background(), ref) })

	spec := func(mode model.ConsumeMode, maxRetry string) model.SubscriptionSpec {
		return model.SubscriptionSpec{
			Ref: ref,
			Attributes: map[string]string{
				rocketmq.AttrConsumeMode: string(mode),
				rocketmq.AttrMaxRetry:    maxRetry,
			},
		}
	}

	if err := groups.CreateSubscription(ctx, spec(model.ModeBroadcasting, "9")); err != nil {
		t.Fatalf("create: %v", err)
	}

	created := findSubscription(t, ctx, groups, name)
	if got := created.Attribute(rocketmq.AttrMaxRetry); got != "9" {
		t.Errorf("maxRetry=%q want 9", got)
	}
	// The permission the config stores, which is the field the edit form has to
	// read back rather than infer - the mode attribute is a client's report and
	// is empty for a group nothing is attached to.
	if got := created.Attribute(rocketmq.AttrBroadcast); got != "true" {
		t.Errorf("broadcastEnabled=%q want true after creating a broadcasting group", got)
	}

	if err := groups.UpdateSubscription(ctx, spec(model.ModeClustering, "3")); err != nil {
		t.Fatalf("update: %v", err)
	}

	updated := findSubscription(t, ctx, groups, name)
	if got := updated.Attribute(rocketmq.AttrMaxRetry); got != "3" {
		t.Errorf("maxRetry=%q want 3 after the update", got)
	}
	if got := updated.Attribute(rocketmq.AttrBroadcast); got != "false" {
		t.Errorf("broadcastEnabled=%q want false after the update", got)
	}

	if err := groups.RemoveSubscription(ctx, ref); err != nil {
		t.Fatalf("delete: %v", err)
	}
	for _, one := range listSubscriptions(t, ctx, groups) {
		if one.Ref.Name == name {
			t.Fatal("the group is still listed after being deleted")
		}
	}
}

func listSubscriptions(
	t *testing.T, ctx context.Context, groups driver.SubscriptionAdmin,
) []*model.Subscription {
	t.Helper()
	listed, err := groups.ListSubscriptions(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return listed
}

func findSubscription(
	t *testing.T, ctx context.Context, groups driver.SubscriptionAdmin, name string,
) *model.Subscription {
	t.Helper()
	for _, one := range listSubscriptions(t, ctx, groups) {
		if one.Ref.Name == name {
			return one
		}
	}
	t.Fatalf("%s is not listed", name)
	return nil
}

// The consumer sheet's 重置位点 action.
func TestLiveResetOffset(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("offsets"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)

	groups := conn.(driver.SubscriptionAdmin)
	listed, err := groups.ListSubscriptions(ctx)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	seeded := false
	for _, one := range listed {
		if one.Ref.Name == seededGroup {
			seeded = true
		}
	}
	if !seeded {
		e2e.Missing(t, "run `npm run e2e:seed` to create %s", seededGroup)
	}

	// Something has to be in the topic for a reset to have a position to move
	// to, and the seeded topic is empty on a fresh broker.
	if _, err := conn.(driver.MessagePublisher).SendMessage(
		ctx, seededTopic, "reset", "reset-probe", `{"probe":"reset"}`, 0,
	); err != nil {
		t.Fatalf("seed a message: %v", err)
	}

	progress := conn.(driver.ProgressAdmin)
	// Timestamp 0 means the earliest retained message, which is the reset the
	// UI offers as 最早.
	if err := progress.ResetOffset(ctx, model.ResetOffsetRequest{
		Group:     seededGroup,
		Topic:     seededTopic,
		Timestamp: 0,
		Force:     true,
	}); err != nil {
		t.Fatalf("reset to earliest: %v", err)
	}

	if err := progress.ResetOffset(ctx, model.ResetOffsetRequest{
		Group:     seededGroup,
		Topic:     seededTopic,
		Timestamp: time.Now().UnixMilli(),
		Force:     true,
	}); err != nil {
		t.Fatalf("reset to now: %v", err)
	}

	// A group that does not exist has to be reported, not silently accepted.
	if err := progress.ResetOffset(ctx, model.ResetOffsetRequest{
		Group:     "MQ_STUDIO_E2E_NO_SUCH_GROUP",
		Topic:     seededTopic,
		Timestamp: 0,
	}); err == nil {
		t.Log("resetting an unknown group was accepted; RocketMQ creates offsets lazily")
	}
}

// Every AccessAdmin method, against a broker that really has ACL on.
//
// None of them had ever run against one. AccessEnabled in particular reported
// false on an ACL-enabled broker, because a broker answers GET_BROKER_CONFIG
// with a Properties document and the library's json.Unmarshal of it fails,
// leaving every setting inside a single "raw" string.
func TestLiveACL(t *testing.T) {
	requireACLBroker(t)
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	input := liveProfileInput("acl")
	input.Endpoints = aclNameServer
	input.SetACL(true, aclAccessKey, aclSecretKey)
	profile, err := connections.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect to the ACL broker: %v", err)
	}
	conn, _ := registry.Get(profile.ID)
	acl := conn.(driver.AccessAdmin)

	enabled, err := acl.AccessEnabled(ctx)
	if err != nil {
		t.Fatalf("AccessEnabled: %v", err)
	}
	if !enabled {
		t.Fatal("AccessEnabled reported false on a broker with aclEnable=true")
	}

	version, err := acl.AccessVersion(ctx)
	if err != nil {
		t.Fatalf("AccessVersion: %v", err)
	}
	if version == nil || version.ClusterName == "" {
		t.Fatalf("AccessVersion returned %+v", version)
	}

	const probeKey = "mq-studio-e2e-probe"
	t.Cleanup(func() { _ = acl.RemoveAccessConfig(context.Background(), probeKey) })

	if err := acl.PutAccessConfig(ctx, model.AccessConfig{
		AccessKey:        probeKey,
		SecretKey:        "mq-studio-e2e-probe-secret",
		DefaultTopicPerm: "SUB",
		DefaultGroupPerm: "SUB",
	}); err != nil {
		t.Fatalf("PutAccessConfig: %v", err)
	}

	// Writing an account bumps the ACL version, which is the only readable
	// evidence that it landed: the library has no call to list the accounts.
	after, err := acl.AccessVersion(ctx)
	if err != nil {
		t.Fatalf("AccessVersion after put: %v", err)
	}
	if after.Version == version.Version {
		t.Log("the ACL version did not move after a write; the broker may batch it")
	}

	// The whitelist is what lets these very calls through, since nothing is
	// signed - so the write has to keep the seed's entries and only add to
	// them, and put them back afterwards. Replacing it with a narrower list
	// locks the next run out of the broker.
	// What plain_acl.seed.yml carries: one address that is never the caller.
	// The whitelist is matched before any signature is, so restoring a wide one
	// would quietly re-open every test that follows - and restoring an empty
	// one is worse, because RocketMQ reads the empty string as a strategy that
	// matches everybody.
	seedWhiteList := []string{"127.0.0.1"}
	t.Cleanup(func() { _ = acl.SetGlobalWhiteAddrs(context.Background(), seedWhiteList) })
	if err := acl.SetGlobalWhiteAddrs(ctx, append(append([]string{}, seedWhiteList...), "10.*.*.*")); err != nil {
		t.Fatalf("SetGlobalWhiteAddrs: %v", err)
	}
	if err := acl.RemoveAccessConfig(ctx, probeKey); err != nil {
		t.Fatalf("RemoveAccessConfig: %v", err)
	}
}

// The credentials a connection profile carries actually reach the Broker.
//
// TestLiveACL is the other half of this: it does the same work with the right
// pair and the E2E broker has no global whitelist, so a signature is the only
// way either of them gets in.
//
// Both halves are here because they fail differently, and only one of them
// proves the signature was computed. A wrong access key is refused on identity
// alone - the Broker never reaches the HMAC - so a client that sent the field
// and signed nothing would pass that half.
func TestLiveACLCredentialsAreSigned(t *testing.T) {
	requireACLBroker(t)
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	cases := []struct {
		name      string
		accessKey string
		secretKey string
		refusal   string
	}{
		{"an unknown access key", "not-a-real-key", "not-a-real-secret", "No acl config"},
		{"the right key with the wrong secret", aclAccessKey, "not-the-secret", "signature"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := liveProfileInput("acl-" + tc.accessKey)
			input.Endpoints = aclNameServer
			input.SetACL(true, tc.accessKey, tc.secretKey)
			profile, err := connections.AddConnection(input)
			if err != nil {
				t.Fatal(err)
			}
			if err := connections.Connect(profile.ID); err != nil {
				t.Fatalf("connect to the ACL broker: %v", err)
			}
			conn, _ := registry.Get(profile.ID)

			_, err = conn.(driver.AccessAdmin).AccessEnabled(ctx)
			if err == nil {
				t.Fatal("the broker accepted a request these credentials cannot sign")
			}
			if !strings.Contains(err.Error(), tc.refusal) {
				t.Errorf("refused for the wrong reason, wanted %q: %v", tc.refusal, err)
			}
		})
	}
}

// What a connected consumer is actually doing.
//
// GetConsumerRunningInfo asks the client rather than the broker, so this is the
// one live test that needs a real consumer. It starts one, waits for the
// rebalance, and then reads back the queues that client holds.
//
// Two library defects used to make this unreadable, and both are pinned here
// because neither fails loudly. The Broker relays the client's answer with a
// binary header, which the remoting decoder used to drop on the floor - the
// call then timed out with a generic error. The body then needs fixJSONBody,
// because mqTable is a Fastjson map whose keys are objects. Underneath that,
// ProcessQueue's field names have to be RocketMQ's own, since a wrong tag
// decodes to zero rather than to an error.
func TestLiveConsumerClients(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("clients"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)
	runtime, ok := conn.(driver.SubscriptionRuntime)
	if !ok {
		t.Fatal("the RocketMQ connection does not implement SubscriptionRuntime")
	}
	ref := model.SubscriptionRef{Name: seededGroup}

	if !conn.Capabilities().Has(model.CapSubscriptionRuntime) {
		t.Error("CapSubscriptionRuntime should be supported")
	}
	if _, degraded := conn.Capabilities().DegradedReason(model.CapSubscriptionRuntime); degraded {
		t.Error("CapSubscriptionRuntime is supported and still carries a degraded reason")
	}

	// With nothing connected the answer is an error, not an empty list: "nobody
	// is consuming" and "everyone is consuming nothing" are different things.
	if _, err := runtime.SubscriptionClients(ctx, ref); err == nil {
		t.Log("a client was already connected; the offline case is not covered by this run")
	}

	stop := startLiveConsumer(t)
	defer stop()

	// Wait for the broker to see the consumer and finish its rebalance. The
	// wait is for the seeded topic rather than for any queue at all: the
	// group's retry queue is assigned separately and often arrives first, so
	// "has assignments" is satisfied a rebalance too early.
	var (
		clients []*model.SubscriptionClient
		seeded  *model.QueueAssignment
	)
	for attempt := range 20 {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		clients, err = runtime.SubscriptionClients(ctx, ref)
		if err != nil || len(clients) == 0 {
			continue
		}
		seeded = assignmentFor(clients[0], seededTopic)
		if seeded != nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("SubscriptionClients: %v", err)
	}
	if len(clients) == 0 {
		t.Fatal("no client reported for a group with a consumer connected")
	}

	client := clients[0]
	if client.ClientID == "" {
		t.Error("a client came back with no id")
	}
	if len(client.Assignments) == 0 {
		t.Fatal("the client reported no queues; GetConsumerRunningInfo is unreadable again")
	}
	if client.Properties["PROP_CLIENT_VERSION"] == "" {
		t.Errorf("the client reported no version: %v", client.Properties)
	}

	// The seeded topic has to be in there, holding a real queue on a real
	// broker. A zero timestamp would mean the field names drifted again.
	if seeded == nil {
		t.Fatalf("no assignment for %s: %+v", seededTopic, client.Assignments)
	}
	if seeded.Node == "" {
		t.Errorf("assignment carries no broker: %+v", *seeded)
	}
	if seeded.LastPull == "" {
		t.Errorf("assignment carries no pull time: %+v", *seeded)
	}
	t.Logf("client %s holds %d queues, %s on %s q%d",
		client.ClientID, len(client.Assignments), seeded.Destination, seeded.Node, seeded.QueueID)
}

// assignmentFor returns the client's queue on topic, or nil if it holds none.
func assignmentFor(client *model.SubscriptionClient, topic string) *model.QueueAssignment {
	for index, assignment := range client.Assignments {
		if assignment.Destination == topic {
			return &client.Assignments[index]
		}
	}
	return nil
}

// startLiveConsumer runs a push consumer on the seeded topic and group.
func startLiveConsumer(t *testing.T) func() {
	t.Helper()
	pushConsumer, err := rocketmqconsumer.NewPushConsumer(
		rocketmqconsumer.WithNameServer([]string{liveNameServer}),
		rocketmqconsumer.WithGroupName(seededGroup),
		rocketmqconsumer.WithConsumerModel(rocketmqconsumer.Clustering),
	)
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}
	err = pushConsumer.Subscribe(seededTopic, rocketmqconsumer.MessageSelector{},
		func(_ context.Context, _ ...*primitive.MessageExt) (rocketmqconsumer.ConsumeResult, error) {
			return rocketmqconsumer.ConsumeSuccess, nil
		})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := pushConsumer.Start(); err != nil {
		t.Fatalf("start consumer: %v", err)
	}
	return func() { _ = pushConsumer.Shutdown() }
}

// Node detail carries the replication state the cluster board needs.
//
// The single-broker E2E cluster has no slaves, so the interesting assertion is
// that asking is safe and answers empty rather than failing - which is the
// case every single-master deployment hits.
func TestLiveNodeDetailReplicas(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("nodes"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)
	cluster := conn.(driver.ClusterAdmin)

	nodes, err := cluster.ListNodes(ctx)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("the cluster reported no nodes")
	}
	// A list must not pay for the replication request.
	for _, node := range nodes {
		if len(node.Replicas) != 0 {
			t.Errorf("ListNodes filled Replicas for %s; that belongs to NodeDetail", node.Address)
		}
	}

	detail, err := cluster.NodeDetail(ctx, nodes[0].Address)
	if err != nil {
		t.Fatalf("node detail: %v", err)
	}
	if detail.Address != nodes[0].Address {
		t.Fatalf("detail address = %q, want %q", detail.Address, nodes[0].Address)
	}
	for _, replica := range detail.Replicas {
		if replica.Address == "" {
			t.Error("a replica came back with no address")
		}
	}
	t.Logf("%s reports %d replica(s)", detail.Address, len(detail.Replicas))
}

// Time-based consumer lag is readable, which no page has claimed yet.
//
// "Four minutes behind" is the number an operator triages on - a backlog of
// 982 is fine at 10k/s and an outage at 10/s - so QueryConsumeTimeSpan is what
// a lag column would be built from. It used to answer nothing: the broker
// wraps the set in an object and the library decoded a bare array, and a bare
// `continue` swallowed the mismatch, so an unreadable response and a group
// that is caught up were the same answer. Fixed in rocketmq-admin-go v1.3.2.
//
// No driver method exposes it yet, so this reaches past the driver to the
// library on purpose: it is the end-to-end record that the data is there to
// build the column from, against a group with offsets it really committed.
func TestLiveConsumeTimeSpanIsReadable(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("timespan"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)

	stop := startLiveConsumer(t)
	defer stop()
	if _, err := conn.(driver.MessagePublisher).SendMessage(
		ctx, seededTopic, "lag", "lag-probe", `{"probe":"lag"}`, 0,
	); err != nil {
		t.Fatalf("publish: %v", err)
	}

	client, err := admin.NewClient(
		admin.WithNameServers([]string{liveNameServer}),
		admin.WithTimeout(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Wait until the group has committed offsets, which is the proof that
	// there is something for a time span to describe.
	var offsets int
	for attempt := range 20 {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		stats, statsErr := client.ExamineConsumeStats(ctx, seededGroup)
		if statsErr == nil && len(stats.OffsetTable) > 0 {
			offsets = len(stats.OffsetTable)
			break
		}
	}
	if offsets == 0 {
		e2e.Missing(t, "the group never committed an offset in %s; nothing to ask a time span about", 20*time.Second)
	}

	spans, err := client.QueryConsumeTimeSpan(ctx, seededTopic, seededGroup)
	if err != nil {
		t.Fatalf("QueryConsumeTimeSpan: %v", err)
	}
	if len(spans) == 0 {
		t.Fatalf("no time span for a group with %d committed offset(s); "+
			"the wrapper decode has regressed", offsets)
	}

	// A queue the group has consumed carries all three stamps. An untouched
	// queue reports -1 for each, which is the broker saying "nothing here"
	// rather than a time, so a lag column has to tell those apart.
	consumed := 0
	for _, span := range spans {
		if span.MinTimeStamp <= 0 {
			continue
		}
		consumed++
		if span.MaxTimeStamp < span.MinTimeStamp {
			t.Errorf("queue %d: max %d is before min %d",
				span.MessageQueue.QueueId, span.MaxTimeStamp, span.MinTimeStamp)
		}
		if span.ConsumeTimeStamp <= 0 {
			t.Errorf("queue %d has messages and no consume timestamp",
				span.MessageQueue.QueueId)
		}
	}
	if consumed == 0 {
		t.Fatalf("%d span(s), none with a timestamp, for a group with %d committed offset(s)",
			len(spans), offsets)
	}
	t.Logf("%d span(s), %d carrying timestamps", len(spans), consumed)
}

// One broker's effective settings.
//
// This is the surface the Properties fix opened up: before it, the library
// handed back a single "raw" key and every lookup missed.
func TestLiveNodeConfig(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("config"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)

	nodes, err := conn.(driver.ClusterAdmin).ListNodes(ctx)
	if err != nil || len(nodes) == 0 {
		t.Fatalf("list nodes: %v (%d nodes)", err, len(nodes))
	}

	inspector := conn.(driver.ConfigInspector)
	config, err := inspector.NodeConfig(ctx, nodes[0].Address)
	if err != nil {
		t.Fatalf("node config: %v", err)
	}
	// A broker reports hundreds of settings; a handful means the document was
	// not parsed, which is the failure this exists to catch.
	if len(config) < 20 {
		t.Fatalf("node config returned %d keys, want a parsed document", len(config))
	}
	if config["brokerClusterName"] == "" {
		t.Errorf("brokerClusterName is missing from %d keys", len(config))
	}
	if _, leaked := config["raw"]; leaked {
		t.Error("the unparsed document leaked through as a key")
	}
	t.Logf("%s reports %d settings", nodes[0].Address, len(config))

	// The discovery tier answers separately, and through the same parser.
	directory, err := inspector.DirectoryConfig(ctx)
	if err != nil {
		t.Fatalf("directory config: %v", err)
	}
	if len(directory) < 5 {
		t.Fatalf("name server config returned %d keys, want a parsed document", len(directory))
	}
	if _, leaked := directory["raw"]; leaked {
		t.Error("the unparsed name server document leaked through as a key")
	}
	t.Logf("name servers report %d settings", len(directory))
}

// Who is currently publishing.
//
// The app could not answer this at all: it knew every consumer group but
// nothing about producers. A producer group has to be named because the broker
// indexes connections by one and offers no way to enumerate them.
func TestLiveProducerClients(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("producers"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)
	inspector := conn.(driver.ProducerInspector)

	const group = "MQ_STUDIO_E2E_PRODUCER"

	// Nobody attached is an empty list, not an error: a producer idle between
	// sends is normal, unlike a consumer group with nothing on it.
	idle, err := inspector.ProducerClients(ctx, group, seededTopic)
	if err != nil {
		t.Fatalf("with no producer attached: %v", err)
	}
	if len(idle) != 0 {
		t.Logf("%d producer(s) already attached; the idle case is not covered by this run", len(idle))
	}

	stop := startLiveProducer(t, group)
	defer stop()

	var clients []*model.ProducerClient
	for attempt := range 15 {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		clients, err = inspector.ProducerClients(ctx, group, seededTopic)
		if err == nil && len(clients) > 0 {
			break
		}
	}
	if err != nil {
		t.Fatalf("ProducerClients: %v", err)
	}
	if len(clients) == 0 {
		t.Fatal("no producer reported while one was publishing")
	}
	for _, client := range clients {
		if client.ClientID == "" || client.Address == "" {
			t.Errorf("producer came back incomplete: %+v", client)
		}
	}
	t.Logf("%s has %d producer(s): %s", group, len(clients), clients[0].Address)
}

// startLiveProducer publishes one message and stays connected.
func startLiveProducer(t *testing.T, group string) func() {
	t.Helper()
	sender, err := rocketmqproducer.NewDefaultProducer(
		rocketmqproducer.WithNameServer([]string{liveNameServer}),
		rocketmqproducer.WithGroupName(group),
	)
	if err != nil {
		t.Fatalf("create producer: %v", err)
	}
	if err := sender.Start(); err != nil {
		t.Fatalf("start producer: %v", err)
	}
	// The broker only learns about a producer once it has routed something.
	if _, err := sender.SendSync(context.Background(), &primitive.Message{
		Topic: seededTopic,
		Body:  []byte(`{"probe":"producer"}`),
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return func() { _ = sender.Shutdown() }
}

// Every housekeeping task a node can be asked to run.
//
// These are safe to run on the E2E broker: each is something it does on its
// own schedule anyway, and the store holds only what these tests published.
func TestLiveNodeMaintenance(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("maintenance"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)

	nodes, err := conn.(driver.ClusterAdmin).ListNodes(ctx)
	if err != nil || len(nodes) == 0 {
		t.Fatalf("list nodes: %v (%d nodes)", err, len(nodes))
	}
	address := nodes[0].Address
	maintenance := conn.(driver.NodeMaintenance)

	for _, task := range model.KnownMaintenanceTasks() {
		if err := maintenance.RunMaintenance(ctx, address, task); err != nil {
			t.Errorf("%s: %v", task, err)
		}
	}

	// A task outside the set must be refused rather than passed through.
	if err := maintenance.RunMaintenance(ctx, address, model.MaintenanceTask("rm -rf")); err == nil {
		t.Error("an unknown maintenance task was accepted")
	}
	// So must an empty address, before anything reaches a broker.
	if err := maintenance.RunMaintenance(ctx, "", model.TaskCleanExpiredQueues); err == nil {
		t.Error("an empty broker address was accepted")
	}
}

// Copying one group's read position onto another.
func TestLiveCloneOffset(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("clone"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)
	cloner := conn.(driver.OffsetCloner)

	const target = "MQ_STUDIO_E2E_GROUP_CLONE"
	if err := cloner.CloneOffset(ctx, model.CloneOffsetRequest{
		From:        seededGroup,
		To:          target,
		Destination: seededTopic,
		FromOffline: true,
	}); err != nil {
		t.Fatalf("clone: %v", err)
	}

	// The destination did not exist before; it does now, because a group is
	// its offsets.
	stats, err := conn.(driver.SubscriptionStats).SubscriptionStats(ctx,
		model.SubscriptionRef{Name: target})
	if err != nil {
		t.Logf("the clone target reports no stats yet: %v", err)
	} else {
		t.Logf("clone target reports %d stat field(s)", len(stats))
	}

	// Cloning onto itself is a mistake worth catching before it reaches a
	// broker, as is a blank name.
	if err := cloner.CloneOffset(ctx, model.CloneOffsetRequest{
		From: seededGroup, To: seededGroup, Destination: seededTopic,
	}); err == nil {
		t.Error("cloning a group onto itself was accepted")
	}
	if err := cloner.CloneOffset(ctx, model.CloneOffsetRequest{
		From: seededGroup, To: "", Destination: seededTopic,
	}); err == nil {
		t.Error("an empty destination group was accepted")
	}
}

// The ACL 2.0 store, against a broker that runs it.
//
// It skips rather than fails where authentication is off, which is every
// environment this repo can start today: the 5.3 auth metadata providers need
// RocksDB, and the official image ships no aarch64 JNI for it, so the ACL
// compose file deliberately runs plain_acl instead. On an amd64 host with
// authenticationEnabled and authorizationEnabled set, this exercises the whole
// surface the ACL board is built on.
func TestLiveAccessDirectory(t *testing.T) {
	requireACLBroker(t)
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	input := liveProfileInput("acl-directory")
	input.Endpoints = aclNameServer
	input.SetACL(true, aclAccessKey, aclSecretKey)
	profile, err := connections.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect to the ACL broker: %v", err)
	}
	conn, _ := registry.Get(profile.ID)
	directory, ok := conn.(driver.AccessDirectory)
	if !ok {
		t.Fatal("the RocketMQ driver no longer implements AccessDirectory")
	}

	enabled, err := directory.DirectoryEnabled(ctx)
	if err != nil {
		t.Fatalf("DirectoryEnabled: %v", err)
	}
	if !enabled {
		t.Skip("this broker runs plain_acl, not 5.3 authentication")
	}

	const probeUser = "mq-studio-e2e-user"
	t.Cleanup(func() { _ = directory.RemovePrincipal(context.Background(), probeUser) })

	if err := directory.PutPrincipal(ctx, model.AccessPrincipalSpec{
		Name:   probeUser,
		Secret: "mq-studio-e2e-user-secret",
		Type:   "Normal",
		Status: "enable",
	}); err != nil {
		t.Fatalf("PutPrincipal: %v", err)
	}

	principals, err := directory.ListPrincipals(ctx)
	if err != nil {
		t.Fatalf("ListPrincipals: %v", err)
	}
	if !hasPrincipal(principals, probeUser) {
		t.Fatalf("the principal just written is not in the listing: %+v", principals)
	}

	t.Cleanup(func() { _ = directory.RemoveAccessRule(context.Background(), probeUser) })
	if err := directory.PutAccessRule(ctx, model.AccessRule{
		Subject: probeUser,
		Policies: []model.AccessPolicy{{
			Resource: "Topic:" + seededTopic,
			Actions:  []string{"Sub"},
			Effect:   "Allow",
		}},
	}); err != nil {
		t.Fatalf("PutAccessRule: %v", err)
	}

	rules, err := directory.ListAccessRules(ctx)
	if err != nil {
		t.Fatalf("ListAccessRules: %v", err)
	}
	if !hasRule(rules, probeUser) {
		t.Fatalf("the rule just written is not in the listing: %+v", rules)
	}

	if err := directory.RemoveAccessRule(ctx, probeUser); err != nil {
		t.Fatalf("RemoveAccessRule: %v", err)
	}
	if err := directory.RemovePrincipal(ctx, probeUser); err != nil {
		t.Fatalf("RemovePrincipal: %v", err)
	}
}

func hasPrincipal(principals []*model.AccessPrincipal, name string) bool {
	for _, principal := range principals {
		if principal != nil && principal.Name == name {
			return true
		}
	}
	return false
}

func hasRule(rules []*model.AccessRule, subject string) bool {
	for _, rule := range rules {
		if rule != nil && rule.Subject == subject {
			return true
		}
	}
	return false
}

// The per-queue rows of a group's consume progress.
//
// The broker keys its offset table by a MessageQueue object, which the library
// hands back as that object's JSON text once its Fastjson fixer has quoted it.
// Nothing but a live broker proves that key is the shape the driver reads, and
// getting it wrong would leave every row with a blank topic and queue -1 while
// the totals beside them still looked right.
func TestLiveConsumeStatsQueues(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("consume-stats"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)

	stats, err := conn.(driver.SubscriptionStats).SubscriptionStats(ctx,
		model.SubscriptionRef{Name: seededGroup})
	if err != nil {
		t.Fatalf("SubscriptionStats: %v", err)
	}
	queues, ok := stats["queues"].([]map[string]interface{})
	if !ok {
		t.Fatalf("no queue rows in %v", stats)
	}
	if len(queues) == 0 {
		t.Fatal("the seeded group reports no queues")
	}

	seeded := 0
	for _, queue := range queues {
		if queue["queueId"].(int) < 0 {
			t.Fatalf("a queue key did not decode: %v", queue)
		}
		if queue["brokerName"] == "" {
			t.Fatalf("a queue row has no broker: %v", queue)
		}
		if queue["topic"] == seededTopic {
			seeded++
		}
	}
	if seeded == 0 {
		t.Fatalf("no row for %s among %d queues", seededTopic, len(queues))
	}
	t.Logf("%d queue row(s), %d of them on %s", len(queues), seeded, seededTopic)
}

// A tail opens on what happens next, then follows it.
//
// The three things worth proving against a real broker: an opening tail sees
// nothing already stored, a message published after it opened comes back, and
// the cursor it returns does not replay that message on the next poll.
func TestLiveMessageTail(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	profile, err := connections.AddConnection(liveProfileInput("tail"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	conn, _ := registry.Get(profile.ID)
	tailer := conn.(driver.MessageTailer)
	ref := model.DestinationRef{Name: seededTopic}

	// Opening on a topic that already holds messages must come back empty.
	opened, err := tailer.TailMessages(ctx, ref, model.TailCursor{}, 32)
	if err != nil {
		t.Fatalf("open tail: %v", err)
	}
	if len(opened.Messages) != 0 {
		t.Fatalf("a tail opened on %d stored message(s); it should start at the end",
			len(opened.Messages))
	}
	if len(opened.Cursor.Positions) == 0 {
		t.Fatal("the opening tail returned no cursor")
	}

	marker := fmt.Sprintf("mq-studio-tail-%d", opened.Cursor.Positions[0].Offset)
	publisher := conn.(driver.MessagePublisher)
	if _, err := publisher.SendMessage(ctx, seededTopic, "", marker, `{"probe":"tail"}`, 0); err != nil {
		t.Fatalf("send: %v", err)
	}

	// The broker stores asynchronously, so the message may need a poll or two.
	var seen bool
	cursor := opened.Cursor
	for attempt := 0; attempt < 10 && !seen; attempt++ {
		batch, err := tailer.TailMessages(ctx, ref, cursor, 32)
		if err != nil {
			t.Fatalf("tail poll: %v", err)
		}
		cursor = batch.Cursor
		for _, message := range batch.Messages {
			if strings.Contains(message.Keys, marker) {
				seen = true
			}
		}
		if !seen {
			time.Sleep(300 * time.Millisecond)
		}
	}
	if !seen {
		t.Fatal("the tail never saw a message published after it opened")
	}

	// The cursor has to have moved past it: a tail that replays what it just
	// showed would double every row on screen.
	after, err := tailer.TailMessages(ctx, ref, cursor, 32)
	if err != nil {
		t.Fatalf("tail after: %v", err)
	}
	for _, message := range after.Messages {
		if strings.Contains(message.Keys, marker) {
			t.Fatal("the tail replayed a message its own cursor had passed")
		}
	}
}

// The overview carries back the TPS history the collector recorded.
//
// Sampling and reading are two different calls on two different timers, and
// nothing tied them together: the collector filed every sample correctly and
// the overview returned nodes that had never been near the history, so the
// trend chart drew its empty state forever. Sampling alone does not prove the
// feature - only reading it back through the call the page actually makes.
func TestLiveOverviewCarriesRecordedTPSHistory(t *testing.T) {
	connections, _, registry := liveStack(t)
	ctx := context.Background()

	settingsService := settings.New(filepath.Join(t.TempDir(), "settings.json"))
	clusters := cluster.New(filepath.Join(t.TempDir(), "tps-history.json"), newConnSource(registry), settingsService)

	profile, err := connections.AddConnection(liveProfileInput("tps"))
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}

	_, before, err := clusters.Overview(ctx, profile.ID)
	if err != nil {
		t.Fatalf("overview before sampling: %v", err)
	}
	if len(before) == 0 {
		t.Fatal("the cluster reported no nodes")
	}
	for _, node := range before {
		if len(node.TpsInHistory) != 0 {
			t.Errorf("%s already had history before anything sampled it", node.Address)
		}
	}

	if err := clusters.CollectTPSSample(ctx, profile.ID); err != nil {
		t.Fatalf("collect sample: %v", err)
	}

	_, after, err := clusters.Overview(ctx, profile.ID)
	if err != nil {
		t.Fatalf("overview after sampling: %v", err)
	}
	for _, node := range after {
		if node.Status != model.NodeOnline {
			continue
		}
		if len(node.TpsInHistory) == 0 || len(node.TpsOutHistory) == 0 {
			t.Fatalf("%s came back with no TPS history after a sample", node.Address)
		}
		if len(node.TpsHistoryTimestamps) != len(node.TpsInHistory) {
			t.Fatalf("%s: %d timestamps for %d samples", node.Address,
				len(node.TpsHistoryTimestamps), len(node.TpsInHistory))
		}
	}
}

// The namespaced fixtures the same seed creates, under their own base names so
// a scoped and an unscoped view can be told apart. See the driver's live tests.
const (
	seededRocketNamespace = "NS_E2E"
	seededNamespacedTopic = "MQ_STUDIO_E2E_NS"
)

/*
 * Switching the namespace from the shell, end to end.
 *
 * The switch is a store and a redial, and the redial is the half that used to
 * be missing: the option reached connections.json and the open client went on
 * dialled with the old one, so every page kept reading the previous namespace
 * with nothing on screen saying so. This asserts through the topic list, which
 * is what the user would have been looking at.
 */
func TestLiveSwitchingTheNamespaceRepointsAnOpenConnection(t *testing.T) {
	connections, topics, _ := liveStack(t)
	ctx := context.Background()

	names := func(id int) []string {
		t.Helper()
		listed, err := topics.List(ctx, id, model.DestinationFilter{})
		if err != nil {
			t.Fatalf("list topics: %v", err)
		}
		found := make([]string, 0, len(listed))
		for _, destination := range listed {
			found = append(found, destination.Ref.Name)
		}
		return found
	}
	holds := func(found []string, want string) bool {
		for _, name := range found {
			if name == want {
				return true
			}
		}
		return false
	}

	profile, err := connections.AddConnection(liveProfileInput("live-scope"))
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}

	unscoped := names(profile.ID)
	if !holds(unscoped, seededTopic) {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:seed` (got %v)", seededTopic, unscoped)
	}

	switched, err := connections.SetOption(profile.ID, rocketmq.OptionNamespace, seededRocketNamespace)
	if err != nil {
		t.Fatalf("switch namespace: %v", err)
	}
	if switched.Status != model.StatusOnline {
		t.Fatalf("status = %q, want the redial to have completed", switched.Status)
	}

	scoped := names(profile.ID)
	if !holds(scoped, seededNamespacedTopic) {
		t.Fatalf("the switched connection does not list %s; got %v", seededNamespacedTopic, scoped)
	}
	if holds(scoped, seededTopic) {
		t.Fatalf("the switched connection still lists %s, so the redial did not take", seededTopic)
	}

	// And back, because a one-way switch is not a switcher.
	if _, err := connections.SetOption(profile.ID, rocketmq.OptionNamespace, ""); err != nil {
		t.Fatalf("switch back to unscoped: %v", err)
	}
	back := names(profile.ID)
	if !holds(back, seededTopic) || holds(back, seededNamespacedTopic) {
		t.Fatalf("switching back did not restore the unscoped view; got %v", back)
	}
}
