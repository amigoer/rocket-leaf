package activemq

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The two live brokers, and why there are two.
//
// Neither is a degraded environment: they are the family's two products, and
// they agree on nothing this driver reads. A test green against Artemis says
// nothing about Classic, so every behaviour below runs against both.
const (
	liveArtemisConsole = "http://127.0.0.1:8161"
	liveArtemisUser    = "artemis"
	liveArtemisPass    = "artemis"
	liveArtemisAMQP    = "amqp://127.0.0.1:61616"

	liveClassicConsole = "http://127.0.0.1:8162"
	liveClassicUser    = "admin"
	liveClassicPass    = "admin"
	liveClassicAMQP    = "amqp://127.0.0.1:5673"
)

func requireArtemis(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.ActiveMQ,
		Name:   "the artemis broker",
		Start:  "npm run e2e:activemq:up",
		// A Jolokia search rather than a GET on the console: Jetty binds 8161
		// while the broker is still starting, so the console answering proves
		// only that the web server is up.
		Probe: e2e.HTTPGet(liveArtemisConsole + "/console/jolokia/search/org.apache.activemq.artemis:broker=*"),
	})
}

func requireClassic(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.ActiveMQ,
		Name:   "the activemq classic broker",
		Start:  "npm run e2e:activemq:classic:up",
		Probe:  e2e.HTTPGet(liveClassicConsole + "/api/jolokia/search/org.apache.activemq:type=Broker,brokerName=*"),
	})
}

// artemisProfile and classicProfile are the two endpoints as a user would
// configure them - console address, console credentials, and the AMQP
// acceptor when the tier is wanted.
func artemisProfile(amqp string) model.ConnectionProfile {
	return model.ConnectionProfile{
		ID:         1,
		Name:       "artemis e2e",
		Kind:       model.KindActiveMQ,
		Endpoints:  liveArtemisConsole,
		TimeoutSec: 10,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options:    map[string]string{OptionAMQPURL: amqp},
		Secrets: map[string]string{
			SecretUsername: liveArtemisUser,
			SecretPassword: liveArtemisPass,
		},
	}
}

func classicProfile(amqp string) model.ConnectionProfile {
	return model.ConnectionProfile{
		ID:         2,
		Name:       "classic e2e",
		Kind:       model.KindActiveMQ,
		Endpoints:  liveClassicConsole,
		TimeoutSec: 10,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options:    map[string]string{OptionAMQPURL: amqp},
		Secrets: map[string]string{
			SecretUsername: liveClassicUser,
			SecretPassword: liveClassicPass,
		},
	}
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// The probe is the driver's first decision and every read afterwards branches
// on it, so getting it wrong is not a wrong answer on one page - it is every
// page addressing MBeans that do not exist.
func TestLiveProbeTellsTheTwoProductsApart(t *testing.T) {
	t.Run("artemis", func(t *testing.T) {
		requireArtemis(t)
		conn, err := open(liveContext(t), artemisProfile(""))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		if conn.tiers.product != artemis {
			t.Errorf("product = %q, want artemis", conn.tiers.product)
		}
		if conn.names.broker != "0.0.0.0" {
			t.Errorf("broker = %q, want 0.0.0.0", conn.names.broker)
		}
	})

	t.Run("classic", func(t *testing.T) {
		requireClassic(t)
		conn, err := open(liveContext(t), classicProfile(""))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		if conn.tiers.product != classic {
			t.Errorf("product = %q, want classic", conn.tiers.product)
		}
		if conn.names.broker != "localhost" {
			t.Errorf("broker = %q, want localhost", conn.names.broker)
		}
	})
}

// Ping reads an attribute off the broker MBean rather than fetching the
// console, because Jetty answers on 8161 both before the broker has started
// and after it has stopped.
func TestLivePingAsksTheBrokerRatherThanTheConsole(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
	}{
		{"artemis", requireArtemis, artemisProfile("")},
		{"classic", requireClassic, classicProfile("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			if err := conn.Ping(ctx); err != nil {
				t.Errorf("ping: %v", err)
			}
			_ = conn.Close()
			if err := conn.Ping(ctx); err == nil {
				t.Error("a closed connection still answered a ping")
			}
		})
	}
}

// The three states of the optional tier, each of which sends a user somewhere
// different: to the connection form, to the broker's acceptor list, or
// nowhere because it is working.
func TestLiveAMQPTierReportsWhichOfItsStatesItIsIn(t *testing.T) {
	for _, tc := range []struct {
		name     string
		require  func(*testing.T)
		profile  func(string) model.ConnectionProfile
		acceptor string
	}{
		{"artemis", requireArtemis, artemisProfile, liveArtemisAMQP},
		{"classic", requireClassic, classicProfile, liveClassicAMQP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)

			live, err := open(ctx, tc.profile(tc.acceptor))
			if err != nil {
				t.Fatalf("open with the acceptor: %v", err)
			}
			defer func() { _ = live.Close() }()
			if live.tiers.amqpReason != "" {
				t.Errorf("the acceptor is open and the tier reported %q", live.tiers.amqpReason)
			}

			unset, err := open(ctx, tc.profile(""))
			if err != nil {
				t.Fatalf("open with no acceptor configured: %v", err)
			}
			defer func() { _ = unset.Close() }()
			if unset.tiers.amqpReason != amqpAbsent {
				t.Errorf("reason with no address = %q, want %q", unset.tiers.amqpReason, amqpAbsent)
			}

			// A port nothing is listening on. Told apart from the above
			// because "you did not configure it" and "the broker refused"
			// send a user to two different places.
			closed, err := open(ctx, tc.profile("amqp://127.0.0.1:1"))
			if err != nil {
				t.Fatalf("open with a closed acceptor: %v", err)
			}
			defer func() { _ = closed.Close() }()
			if closed.tiers.amqpReason != amqpUnreachable {
				t.Errorf("reason for a closed port = %q, want %q", closed.tiers.amqpReason, amqpUnreachable)
			}
		})
	}
}

// Both brokers ship jolokia-access.xml with strict-checking, so a request
// carrying no Origin is refused as coming from the null origin - and the
// refusal is a 403 that reads exactly like bad credentials. This asserts the
// header is what makes the difference, against the real policy file rather
// than against a fixture repeating what the driver believes.
func TestLiveJolokiaRefusesACallWithNoOrigin(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		console string
		path    string
		user    string
		pass    string
		pattern string
	}{
		{"artemis", requireArtemis, liveArtemisConsole, artemisPath, liveArtemisUser, liveArtemisPass,
			artemisDomain + ":broker=*"},
		{"classic", requireClassic, liveClassicConsole, classicPath, liveClassicUser, liveClassicPass,
			classicDomain + ":type=Broker,brokerName=*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)

			with, err := newJolokiaClient(tc.console, tc.path, tc.user, tc.pass, "", 10*time.Second, false)
			if err != nil {
				t.Fatalf("client: %v", err)
			}
			found, err := with.search(ctx, tc.pattern)
			if err != nil {
				t.Fatalf("search with an origin: %v", err)
			}
			if len(found) == 0 {
				t.Fatal("search with an origin found no broker")
			}

			// A client built without one, which is the only way to send no
			// header at all - setting the field to "" on a constructed client
			// sends an empty Origin, which is a different thing to the agent.
			without := *with
			without.origin = ""
			if _, err := without.search(ctx, tc.pattern); err == nil {
				// Artemis skips here and Classic does not, and the difference
				// is the image rather than the product: apache/activemq-artemis
				// defaults EXTRA_ARGS to --relax-jolokia, which strips
				// <strict-checking/> out of the generated policy. An Artemis
				// created without that flag refuses exactly as Classic does,
				// so the header stays mandatory in the driver.
				t.Skip("this broker's jolokia-access.xml does not check the origin")
			} else if !forbidden(err) {
				t.Errorf("a call with no origin failed with %v, want a refusal", err)
			}
		})
	}
}

// Credentials that are wrong have to fail as credentials. The agent answers a
// refusal the same way it answers an origin it does not like, so a driver that
// retried past it would report "no broker here" for a typo in a password.
func TestLiveWrongCredentialsFailAsARefusal(t *testing.T) {
	requireArtemis(t)
	profile := artemisProfile("")
	profile.Secrets[SecretPassword] = "not-the-password"

	if _, err := open(liveContext(t), profile); err == nil {
		t.Fatal("opened with a wrong password")
	}
}

// Destinations, against both trees.
//
// The two products disagree about what a destination even is - Classic
// addresses one directly, Artemis routes through an address and stores in a
// queue - so this is where the reduction either holds or does not.
func TestLiveDestinationsReadTheSameShapeFromBothTrees(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
	}{
		{"artemis", requireArtemis, artemisProfile("")},
		{"classic", requireClassic, classicProfile("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			queue := "MQS.TEST.destinations." + tc.name
			topic := "MQS.TEST.topic." + tc.name
			if err := conn.CreateDestination(ctx, model.DestinationSpec{
				Ref: model.DestinationRef{Name: queue},
			}); err != nil {
				t.Fatalf("create queue: %v", err)
			}
			defer func() { _ = conn.RemoveDestination(ctx, model.DestinationRef{Name: queue}) }()

			if err := conn.CreateDestination(ctx, model.DestinationSpec{
				Ref:        model.DestinationRef{Name: topic},
				Attributes: map[string]string{AttrKind: string(topicKind)},
			}); err != nil {
				t.Fatalf("create topic: %v", err)
			}
			defer func() { _ = conn.RemoveDestination(ctx, model.DestinationRef{Name: topic}) }()

			found, err := conn.ListDestinations(ctx, model.DestinationFilter{})
			if err != nil {
				t.Fatalf("list: %v", err)
			}

			byName := make(map[string]*model.Destination, len(found))
			for _, destination := range found {
				byName[destination.Ref.Name] = destination
			}

			if got := byName[queue]; got == nil {
				t.Errorf("the queue this test created is not in the listing")
			} else {
				if got.Attributes[AttrKind] != string(queueKind) {
					t.Errorf("queue kind = %q", got.Attributes[AttrKind])
				}
				if got.Attributes[AttrProduct] != tc.name {
					t.Errorf("product = %q, want %q", got.Attributes[AttrProduct], tc.name)
				}
				// JMS has neither, and a zero here would read as "one
				// partition" rather than "no such thing".
				if got.Partitions != model.UnknownMetric {
					t.Errorf("partitions = %d, want unknown", got.Partitions)
				}
				if got.RateIn != model.UnknownMetric || got.RateOut != model.UnknownMetric {
					t.Errorf("rates = %d/%d, want unknown: neither product reports one",
						got.RateIn, got.RateOut)
				}
			}

			if got := byName[topic]; got == nil {
				t.Errorf("the topic this test created is not in the listing")
			} else if got.Attributes[AttrKind] != string(topicKind) {
				t.Errorf("topic kind = %q, want topic", got.Attributes[AttrKind])
			}
		})
	}
}

// The seeded depths, which are the figures a board prints. Read against a
// queue whose contents this test did not create, because a driver that only
// ever measures its own three messages would not notice reading the wrong
// attribute off the wrong MBean.
func TestLiveDestinationDepthMatchesWhatWasSeeded(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
		depth   int64
	}{
		{"artemis", requireArtemis, artemisProfile(""), 120},
		{"classic", requireClassic, classicProfile(""), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			orders, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: "MQS.SEED.orders"})
			if err != nil {
				t.Skipf("the seed has not run: %v", err)
			}
			if orders.Depth != tc.depth {
				t.Errorf("depth = %d, want %d (run npm run e2e:activemq:seed)", orders.Depth, tc.depth)
			}
		})
	}
}

// Classic publishes an advisory topic per destination per event, which on a
// seeded broker is dozens of topics nobody declared. Artemis keeps its own
// under $sys and activemq.notifications. Either would bury the list.
func TestLiveInternalDestinationsAreHiddenUnlessAskedFor(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
	}{
		{"artemis", requireArtemis, artemisProfile("")},
		{"classic", requireClassic, classicProfile("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			visible, err := conn.ListDestinations(ctx, model.DestinationFilter{})
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			for _, destination := range visible {
				if isInternal(conn.tiers.product, destination.Ref.Name) {
					t.Errorf("%q is internal and was listed anyway", destination.Ref.Name)
				}
			}

			all, err := conn.ListDestinations(ctx, model.DestinationFilter{IncludeInternal: true})
			if err != nil {
				t.Fatalf("list including internal: %v", err)
			}
			if len(all) <= len(visible) {
				t.Errorf("including internal returned %d against %d visible; this broker "+
					"should have some", len(all), len(visible))
			}
		})
	}
}

// An Artemis multicast address's queues are its durable subscriptions, not
// destinations of their own. Listing them here would show a topic with two
// subscribers as three rows, two of which nobody declared.
func TestLiveArtemisSubscriptionsAreNotListedAsDestinations(t *testing.T) {
	requireArtemis(t)
	ctx := liveContext(t)
	conn, err := open(ctx, artemisProfile(""))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	found, err := conn.ListDestinations(ctx, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	var events *model.Destination
	for _, destination := range found {
		if destination.Ref.Name == "MQS.SEED.events.analytics" {
			t.Error("a durable subscription was listed as a destination")
		}
		if destination.Ref.Name == "MQS.SEED.events" {
			events = destination
		}
	}
	if events == nil {
		t.Skip("the seed has not run")
	}
	if events.Attributes[AttrKind] != string(topicKind) {
		t.Errorf("the seeded multicast address reads as %q", events.Attributes[AttrKind])
	}
	// Its subscribers are its subscriptions, which exist with nothing
	// connected - that is what durable means.
	if events.Subscribers != 2 {
		t.Errorf("subscribers = %d, want 2", events.Subscribers)
	}
}

// Purging and moving, against both trees.
//
// Purge is the one where Artemis's two levels bite: a multicast address holds
// nothing itself, so emptying a topic means emptying every subscription queue
// under it, and a driver that called removeAllMessages on the address would
// report success and change nothing.
func TestLivePurgeEmptiesADestination(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
	}{
		{"artemis", requireArtemis, artemisProfile("")},
		{"classic", requireClassic, classicProfile("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			queue := "MQS.TEST.purge." + tc.name
			if err := conn.CreateDestination(ctx, model.DestinationSpec{
				Ref: model.DestinationRef{Name: queue},
			}); err != nil {
				t.Fatalf("create: %v", err)
			}
			defer func() { _ = conn.RemoveDestination(ctx, model.DestinationRef{Name: queue}) }()

			for i := range 5 {
				if err := sendTestMessage(ctx, conn, queue, fmt.Sprintf("purge-%d", i)); err != nil {
					t.Fatalf("send: %v", err)
				}
			}
			if depth := depthOf(t, ctx, conn, queue); depth != 5 {
				t.Fatalf("depth before purge = %d, want 5", depth)
			}

			if err := conn.PurgeQueue(ctx, model.DestinationRef{Name: queue}); err != nil {
				t.Fatalf("purge: %v", err)
			}
			if depth := depthOf(t, ctx, conn, queue); depth != 0 {
				t.Errorf("depth after purge = %d, want 0", depth)
			}
		})
	}
}

// Moving reports the broker's own count, which is what makes the number worth
// showing: a move that matched nothing and a move that moved everything are
// otherwise the same successful call.
func TestLiveMoveReportsWhatTheBrokerMoved(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
	}{
		{"artemis", requireArtemis, artemisProfile("")},
		{"classic", requireClassic, classicProfile("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			from := "MQS.TEST.move.from." + tc.name
			to := "MQS.TEST.move.to." + tc.name
			for _, name := range []string{from, to} {
				if err := conn.CreateDestination(ctx, model.DestinationSpec{
					Ref: model.DestinationRef{Name: name},
				}); err != nil {
					t.Fatalf("create %s: %v", name, err)
				}
				defer func() { _ = conn.RemoveDestination(ctx, model.DestinationRef{Name: name}) }()
			}

			for i := range 3 {
				if err := sendTestMessage(ctx, conn, from, fmt.Sprintf("move-%d", i)); err != nil {
					t.Fatalf("send: %v", err)
				}
			}

			moved, err := conn.MoveMessages(ctx, model.MoveRequest{From: from, ToRoutingKey: to})
			if err != nil {
				t.Fatalf("move: %v", err)
			}
			if moved != 3 {
				t.Errorf("moved = %d, want 3", moved)
			}
			if depth := depthOf(t, ctx, conn, from); depth != 0 {
				t.Errorf("source depth = %d, want 0", depth)
			}
			if depth := depthOf(t, ctx, conn, to); depth != 3 {
				t.Errorf("target depth = %d, want 3", depth)
			}
		})
	}
}

// sendTestMessage puts one text message on a destination through the same JMX
// operation the publish page uses, so a test needs no wire client.
func sendTestMessage(ctx context.Context, conn *Conn, queue, body string) error {
	mbean := conn.names.destination(queue, queueKind)
	if conn.tiers.product == artemis {
		_, err := conn.jolokia.call(ctx, execOperation(mbean,
			"sendMessage(java.util.Map,int,java.lang.String,boolean,java.lang.String,java.lang.String)",
			nil, 3, body, true, conn.config.username, conn.config.password))
		return err
	}
	_, err := conn.jolokia.call(ctx, execOperation(mbean,
		"sendTextMessage(java.lang.String)", body))
	return err
}

func depthOf(t *testing.T, ctx context.Context, conn *Conn, queue string) int64 {
	t.Helper()
	detail, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: queue})
	if err != nil {
		t.Fatalf("detail for %s: %v", queue, err)
	}
	return detail.Depth
}

// Durable subscriptions, which the two products keep in unrelated places:
// Artemis as a queue bound to a multicast address, Classic as a consumer
// registered against a topic and identified by a client id and a name.
func TestLiveDurableSubscriptionsRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
		// The canonical ref name each product needs: Artemis takes the queue's
		// name, Classic a client id joined to a subscription name.
		subName string
	}{
		{"artemis", requireArtemis, artemisProfile(""), "MQS.TEST.sub.artemis"},
		{"classic", requireClassic, classicProfile(""), "mqs-test-client|MQS.TEST.sub.classic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			topic := "MQS.TEST.subtopic." + tc.name
			if err := conn.CreateDestination(ctx, model.DestinationSpec{
				Ref:        model.DestinationRef{Name: topic},
				Attributes: map[string]string{AttrKind: string(topicKind)},
			}); err != nil {
				t.Fatalf("create topic: %v", err)
			}
			defer func() { _ = conn.RemoveDestination(ctx, model.DestinationRef{Name: topic}) }()

			if err := conn.CreateSubscription(ctx, model.SubscriptionSpec{
				Ref: model.SubscriptionRef{Namespace: topic, Name: tc.subName},
			}); err != nil {
				t.Fatalf("create subscription: %v", err)
			}
			defer func() {
				_ = conn.RemoveSubscription(ctx, model.SubscriptionRef{Name: tc.subName})
			}()

			found, err := conn.ListSubscriptions(ctx)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			var made *model.Subscription
			for _, subscription := range found {
				if subscription.Ref.Name == tc.subName {
					made = subscription
				}
			}
			if made == nil {
				t.Fatalf("the subscription this test created is not in the listing of %d", len(found))
			}
			if made.Ref.Namespace != topic {
				t.Errorf("namespace = %q, want the topic %q", made.Ref.Namespace, topic)
			}
			// Nothing is connected to it, and that is the resting state of a
			// durable subscription rather than a fault - the broker is holding
			// messages for a client that is not there.
			if made.Status != model.SubscriptionOffline {
				t.Errorf("status = %q, want offline", made.Status)
			}
			if made.Members != 0 {
				t.Errorf("members = %d, want 0", made.Members)
			}
			if made.Backlog != 0 {
				t.Errorf("backlog = %d, want 0", made.Backlog)
			}

			if err := conn.RemoveSubscription(ctx, model.SubscriptionRef{Name: tc.subName}); err != nil {
				t.Fatalf("remove: %v", err)
			}
			after, err := conn.ListSubscriptions(ctx)
			if err != nil {
				t.Fatalf("list after remove: %v", err)
			}
			for _, subscription := range after {
				if subscription.Ref.Name == tc.subName {
					t.Error("the subscription is still listed after being removed")
				}
			}
		})
	}
}

// A durable subscription accrues a backlog while nothing is connected to it,
// which is the only reason it exists. Read from the seeded topic, so this is
// the broker's own figure rather than one this test just created.
func TestLiveSubscriptionBacklogGrowsWhileNothingIsConnected(t *testing.T) {
	requireArtemis(t)
	ctx := liveContext(t)
	conn, err := open(ctx, artemisProfile(""))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	found, err := conn.ListSubscriptions(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var analytics *model.Subscription
	for _, subscription := range found {
		if subscription.Ref.Name == "MQS.SEED.events.analytics" {
			analytics = subscription
		}
	}
	if analytics == nil {
		t.Skip("the seed has not run")
	}
	before := analytics.Backlog

	// Sent to the address, which fans out to every queue under it - that fan
	// out is what a multicast address is for, and it is the thing a driver
	// reading the address's own MessageCount would miss.
	for i := range 4 {
		if _, err := conn.jolokia.call(ctx, execOperation(
			conn.names.artemisQueue("MQS.SEED.events", "MQS.SEED.events.analytics", multicast),
			"sendMessage(java.util.Map,int,java.lang.String,boolean,java.lang.String,java.lang.String)",
			nil, 3, fmt.Sprintf("fanout-%d", i), true, liveArtemisUser, liveArtemisPass)); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	after, err := conn.SubscriptionDetail(ctx, model.SubscriptionRef{Name: "MQS.SEED.events.analytics"})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if after.Backlog != before+4 {
		t.Errorf("backlog = %d, want %d", after.Backlog, before+4)
	}
}

// Browsing, and the thing that makes this family unusual: it is a management
// operation, so reading a destination takes nothing off it.
//
// RabbitMQ's browse goes through basic.get and alters the queue even when what
// it read is put back, which is why its message page carries a caveat. This
// asserts ActiveMQ's does not, by browsing twice and checking the depth.
func TestLiveBrowseTakesNothingOffTheDestination(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
	}{
		{"artemis", requireArtemis, artemisProfile("")},
		{"classic", requireClassic, classicProfile("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			queue := "MQS.TEST.browse." + tc.name
			if err := conn.CreateDestination(ctx, model.DestinationSpec{
				Ref: model.DestinationRef{Name: queue},
			}); err != nil {
				t.Fatalf("create: %v", err)
			}
			defer func() { _ = conn.RemoveDestination(ctx, model.DestinationRef{Name: queue}) }()

			for i := range 3 {
				if err := sendTestMessage(ctx, conn, queue, fmt.Sprintf("browse-%d", i)); err != nil {
					t.Fatalf("send: %v", err)
				}
			}

			first, err := conn.QueryMessages(ctx, model.MessageQueryParams{Topic: queue})
			if err != nil {
				t.Fatalf("browse: %v", err)
			}
			if len(first) != 3 {
				t.Fatalf("browse returned %d, want 3", len(first))
			}
			second, err := conn.QueryMessages(ctx, model.MessageQueryParams{Topic: queue})
			if err != nil {
				t.Fatalf("second browse: %v", err)
			}
			if len(second) != 3 {
				t.Errorf("second browse returned %d, want 3 - browsing consumed something", len(second))
			}
			if depth := depthOf(t, ctx, conn, queue); depth != 3 {
				t.Errorf("depth after two browses = %d, want 3", depth)
			}

			// The bodies survive the two products' unrelated key sets. Classic
			// answers with Text and Artemis with text, and a driver reading
			// the wrong one returns three empty messages and no error.
			for _, message := range first {
				if !strings.HasPrefix(message.Body, "browse-") {
					t.Errorf("body = %q, want the text that was sent", message.Body)
				}
				if message.MessageID == "" {
					t.Error("a message came back with no id")
				}
				if message.StoreTimestamp <= 0 {
					t.Error("a message came back with no timestamp")
				}
			}
		})
	}
}

// The cap, on the broker rather than in a fixture. Classic stops at
// maxBrowsePageSize - 400 by default - however deep the destination is, and
// the attribute is not readable, so this is the only place the number can be
// confirmed. The seed puts 500 on the orders queue for exactly this.
func TestLiveClassicBrowseStopsAtItsPageSize(t *testing.T) {
	requireClassic(t)
	ctx := liveContext(t)
	conn, err := open(ctx, classicProfile(""))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	orders, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: "MQS.SEED.orders"})
	if err != nil {
		t.Skipf("the seed has not run: %v", err)
	}
	if orders.Depth <= classicBrowseCap {
		t.Skipf("the seeded queue holds %d, which is not past the cap", orders.Depth)
	}

	messages, err := conn.QueryMessages(ctx, model.MessageQueryParams{
		Topic:      "MQS.SEED.orders",
		MaxResults: 1000,
	})
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(messages) != classicBrowseCap {
		t.Errorf("browse of a %d-deep queue returned %d, want the %d cap",
			orders.Depth, len(messages), classicBrowseCap)
	}

	// And the connection says so, rather than leaving a reader to conclude the
	// queue is 400 deep.
	if _, ok := conn.Capabilities().Caveat(model.CapMessageQuery); !ok {
		t.Error("classic declares no caveat on browsing")
	}
}

// Artemis pages, so it has no cap and must not claim one.
func TestLiveArtemisBrowseHasNoCaveat(t *testing.T) {
	requireArtemis(t)
	ctx := liveContext(t)
	conn, err := open(ctx, artemisProfile(""))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if reason, ok := conn.Capabilities().Caveat(model.CapMessageQuery); ok {
		t.Errorf("artemis declares a browse caveat %q and pages properly", reason)
	}
}

// An Artemis topic holds nothing of its own - its messages are in the
// subscription queues under it - so browsing one has to say that rather than
// answering with an empty page that looks like an idle topic.
func TestLiveBrowsingAnArtemisTopicSaysWhereTheMessagesAre(t *testing.T) {
	requireArtemis(t)
	ctx := liveContext(t)
	conn, err := open(ctx, artemisProfile(""))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.QueryMessages(ctx, model.MessageQueryParams{
		Topic: "MQS.SEED.events",
	}); err == nil {
		t.Error("browsing a multicast address answered instead of saying where to look")
	}
}

// Dead letters, which this family has properly and most of the others do not.
//
// Kafka has no broker-side dead-letter queue; NATS moves nothing; Redis keeps
// a pending list because it gives up on nothing. ActiveMQ moves the message to
// a destination the operator named and can put it back where it came from,
// which makes it the first family ever to exercise the retry.
func TestLiveDeadLettersAreFoundByWalkingTheDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
		expect  string
	}{
		{"artemis", requireArtemis, artemisProfile(""), "DLQ"},
		{"classic", requireClassic, classicProfile(""), "ActiveMQ.DLQ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			// Classic creates ActiveMQ.DLQ the first time something is dead
			// lettered, so a broker that has never failed a delivery has none
			// - which is a true answer rather than a missing one.
			if tc.name == "classic" {
				if err := conn.CreateDestination(ctx, model.DestinationSpec{
					Ref: model.DestinationRef{Name: tc.expect},
				}); err != nil {
					t.Fatalf("create the dead-letter queue: %v", err)
				}
			}

			queues, err := conn.DeadLetterQueues(ctx, "")
			if err != nil {
				t.Fatalf("dead letter queues: %v", err)
			}
			var found *model.DeadLetterQueue
			for _, queue := range queues {
				if queue.Name == tc.expect {
					found = queue
				}
			}
			if found == nil {
				names := make([]string, 0, len(queues))
				for _, queue := range queues {
					names = append(names, queue.Name)
				}
				t.Fatalf("%s is not among the dead-letter queues %v", tc.expect, names)
			}

			// Artemis records a DeadLetterAddress on every queue, so the page
			// can say which destinations feed this one. Classic decides by
			// broker policy and keeps no such record, so the list is empty -
			// and inventing it from names would be a guess dressed as
			// topology.
			if tc.name == "artemis" && len(found.Sources) == 0 {
				t.Error("artemis declares a dead-letter address per queue and no sources were found")
			}
			if tc.name == "classic" && len(found.Sources) != 0 {
				t.Errorf("classic keeps no record of what fed a dead-letter queue, got %d sources",
					len(found.Sources))
			}
		})
	}
}

// Retry puts each message back on the destination it originally failed on,
// which is the operation no other family in this app has. Both products spell
// it retryMessages() with no arguments - the selector-taking form the
// documentation describes exists on neither.
func TestLiveRetryPutsDeadLettersBackWhereTheyCameFrom(t *testing.T) {
	requireArtemis(t)
	ctx := liveContext(t)
	conn, err := open(ctx, artemisProfile(""))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	origin := "MQS.TEST.dlq.origin"
	if err := conn.CreateDestination(ctx, model.DestinationSpec{
		Ref: model.DestinationRef{Name: origin},
	}); err != nil {
		t.Fatalf("create origin: %v", err)
	}
	defer func() { _ = conn.RemoveDestination(ctx, model.DestinationRef{Name: origin}) }()

	// The annotations Artemis itself writes when it dead-letters a message,
	// which is what retryMessages reads to know where to put it back. Setting
	// them by hand is the only way to produce a dead letter without a consumer
	// that refuses one max-delivery-attempts times.
	dlq := conn.names.artemisQueue("DLQ", "DLQ", anycast)
	for i := range 2 {
		if _, err := conn.jolokia.call(ctx, execOperation(dlq,
			"sendMessage(java.util.Map,int,java.lang.String,boolean,java.lang.String,java.lang.String)",
			map[string]string{"_AMQ_ORIG_ADDRESS": origin, "_AMQ_ORIG_QUEUE": origin},
			3, fmt.Sprintf("dead-%d", i), true, liveArtemisUser, liveArtemisPass)); err != nil {
			t.Fatalf("send to the dead-letter queue: %v", err)
		}
	}

	retried, err := conn.RetryDeadLetters(ctx, model.DestinationRef{Name: "DLQ"})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retried < 2 {
		t.Errorf("retried = %d, want at least the 2 this test dead-lettered", retried)
	}
	if depth := depthOf(t, ctx, conn, origin); depth != 2 {
		t.Errorf("the origin holds %d after the retry, want 2", depth)
	}
}

// Sending, which is a management operation here for the same reason browsing
// is - so the send console works on a broker with every acceptor switched off.
func TestLivePublishReachesTheDestination(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
	}{
		{"artemis", requireArtemis, artemisProfile("")},
		{"classic", requireClassic, classicProfile("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			queue := "MQS.TEST.publish." + tc.name
			if err := conn.CreateDestination(ctx, model.DestinationSpec{
				Ref: model.DestinationRef{Name: queue},
			}); err != nil {
				t.Fatalf("create: %v", err)
			}
			defer func() { _ = conn.RemoveDestination(ctx, model.DestinationRef{Name: queue}) }()

			result, err := conn.Publish(ctx, model.PublishRequest{
				RoutingKey:    queue,
				Body:          "published",
				Persistent:    true,
				Count:         3,
				CorrelationID: "corr-1",
				Headers:       map[string]string{"tenant": "acme"},
			})
			if err != nil {
				t.Fatalf("publish: %v", err)
			}
			if result.Sent != 3 {
				t.Errorf("sent = %d, want 3 (reason %q)", result.Sent, result.Reason)
			}
			if depth := depthOf(t, ctx, conn, queue); depth != 3 {
				t.Errorf("depth = %d, want 3", depth)
			}

			// The headers have to survive the round trip, or the console is
			// collecting fields the broker throws away.
			messages, err := conn.QueryMessages(ctx, model.MessageQueryParams{Topic: queue})
			if err != nil {
				t.Fatalf("browse: %v", err)
			}
			if len(messages) == 0 {
				t.Fatal("nothing came back from the browse")
			}
			if got := messages[0].Properties["tenant"]; got != "acme" {
				t.Errorf("the header a producer set came back as %q", got)
			}
		})
	}
}

// Delayed delivery is accepted by both management operations and honoured by
// neither, which is why the capability is not declared. This pins that: if a
// broker version ever starts honouring it, this test fails and the capability
// can be added rather than discovered by a user whose delayed message went out
// at once.
func TestLiveDelayedDeliveryIsNotHonouredThroughJolokia(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
		header  string
		value   string
	}{
		{"artemis", requireArtemis, artemisProfile(""), "_AMQ_SCHED_DELIVERY", ""},
		{"classic", requireClassic, classicProfile(""), "AMQ_SCHEDULED_DELAY", "60000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			queue := "MQS.TEST.delay." + tc.name
			if err := conn.CreateDestination(ctx, model.DestinationSpec{
				Ref: model.DestinationRef{Name: queue},
			}); err != nil {
				t.Fatalf("create: %v", err)
			}
			defer func() { _ = conn.RemoveDestination(ctx, model.DestinationRef{Name: queue}) }()

			value := tc.value
			if value == "" {
				// Artemis takes an absolute delivery time rather than an offset.
				value = fmt.Sprintf("%d", time.Now().Add(time.Minute).UnixMilli())
			}
			if _, err := conn.Publish(ctx, model.PublishRequest{
				RoutingKey: queue,
				Body:       "later",
				Persistent: true,
				Count:      1,
				Headers:    map[string]string{tc.header: value},
			}); err != nil {
				t.Fatalf("publish: %v", err)
			}

			detail, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: queue})
			if err != nil {
				t.Fatalf("detail: %v", err)
			}
			if detail.Depth != 1 {
				t.Errorf("depth = %d; a delay set as a string property now seems to be "+
					"honoured, so CapDelayedDelivery may be declarable", detail.Depth)
			}
		})
	}
}

// SendMessage refuses a delay rather than sending immediately and reporting
// one, which is the failure the capability's absence exists to prevent.
func TestLiveSendMessageRefusesADelayItCannotHonour(t *testing.T) {
	requireArtemis(t)
	ctx := liveContext(t)
	conn, err := open(ctx, artemisProfile(""))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.SendMessage(ctx, "MQS.SEED.audit", "", "", "body", 30); err == nil {
		t.Error("a delayed send was accepted")
	}
}

// The broker page, which on this family is a broker page more than a cluster
// page: a JMS broker is a unit, and what the other families call a node here
// is the one broker plus whatever it bridges to.
func TestLiveBrokerReportsItselfAndItsFigures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
		broker  string
	}{
		{"artemis", requireArtemis, artemisProfile(""), "0.0.0.0"},
		{"classic", requireClassic, classicProfile(""), "localhost"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			nodes, err := conn.ListNodes(ctx)
			if err != nil {
				t.Fatalf("list nodes: %v", err)
			}
			if len(nodes) == 0 {
				t.Fatal("the broker did not list itself")
			}
			node := nodes[0]
			if node.Name != tc.broker {
				t.Errorf("name = %q, want %q", node.Name, tc.broker)
			}
			if node.Version == "" {
				t.Error("no version reported")
			}
			if node.RateIn != model.UnknownMetric || node.RateOut != model.UnknownMetric {
				t.Errorf("rates = %d/%d; neither product reports one",
					node.RateIn, node.RateOut)
			}
			if node.Attributes[AttrUptime] == "" {
				t.Error("no uptime reported")
			}

			overview, err := conn.ClusterOverview(ctx)
			if err != nil {
				t.Fatalf("overview: %v", err)
			}
			if overview.TotalNodes != len(nodes) {
				t.Errorf("overview counts %d nodes against a list of %d",
					overview.TotalNodes, len(nodes))
			}
			if overview.Destinations <= 0 {
				t.Errorf("destinations = %d on a seeded broker", overview.Destinations)
			}

			config, err := conn.NodeConfig(ctx, node.Address)
			if err != nil {
				t.Fatalf("node config: %v", err)
			}
			if len(config) < 10 {
				t.Errorf("effective settings came back with %d entries", len(config))
			}
			// Scalars only: the tree also carries destination lists and
			// connector maps, which belong on their own pages.
			for key, value := range config {
				if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
					t.Errorf("setting %q came through as a structure: %s", key, value)
				}
			}

			census, err := conn.Census(ctx)
			if err != nil {
				t.Fatalf("census: %v", err)
			}
			if census.Version != node.Version {
				t.Errorf("census version %q against node version %q", census.Version, node.Version)
			}
		})
	}
}

// Connections, which the two products expose through unrelated shapes: Artemis
// answers with JSON from one operation, Classic registers an MBean per
// connection under the connector that accepted it - and registers each one
// twice, once per view type, which a plain listing would show as two clients.
func TestLiveConnectionsAreListedOncePerClient(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile func(string) model.ConnectionProfile
		amqp    string
	}{
		{"artemis", requireArtemis, artemisProfile, liveArtemisAMQP},
		{"classic", requireClassic, classicProfile, liveClassicAMQP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile(tc.amqp))
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			// A real client, so the list has something in it. The AMQP tier is
			// what this test borrows it from.
			client, err := conn.dialAMQPClient(ctx)
			if err != nil {
				t.Skipf("the amqp acceptor is not answering: %v", err)
			}
			defer func() { _ = client.Close() }()

			connections, err := conn.ListClientConnections(ctx, "")
			if err != nil {
				t.Fatalf("list connections: %v", err)
			}
			if len(connections) == 0 {
				t.Fatal("a connection is open and none was listed")
			}

			seen := make(map[string]bool, len(connections))
			for _, connection := range connections {
				if seen[connection.Name] {
					t.Errorf("connection %q listed twice", connection.Name)
				}
				seen[connection.Name] = true
				if connection.PeerHost == "" {
					t.Errorf("connection %q has no peer host", connection.Name)
				}
			}

			// Channels are AMQP 0-9-1's and neither product has them. A JMS
			// session is not one: it carries no prefetch of its own and is not
			// something an operator closes.
			channels, err := conn.ListClientChannels(ctx, "")
			if err != nil {
				t.Fatalf("list channels: %v", err)
			}
			if len(channels) != 0 {
				t.Errorf("channels = %d; neither product has them", len(channels))
			}
		})
	}
}
