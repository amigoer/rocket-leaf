package ibmmq

import (
	"context"
	"crypto/tls"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The live queue manager, and the two accounts the developer image ships.
//
// They are two because the mqweb server maps its two roles to two groups and
// the image puts one user in each: admin holds MQWebAdmin and reaches the
// administrative interface, app holds MQWebUser and reaches the messaging one.
// Neither can do the other's work, which is what makes this environment able
// to exercise both halves of the driver's tier split rather than only the
// happy path.
const (
	liveMQWeb        = "https://127.0.0.1:9443"
	liveQueueManager = "QM1"

	liveAdminUser = "admin"
	liveAdminPass = "passw0rd"
	liveAppUser   = "app"
	liveAppPass   = "passw0rd"
)

// Objects the seed made, which the live tests read and never change. Anything
// a test creates is named MQS.TEST.* so the two can never collide.
const (
	seedQueue        = "MQS.SEED.ORDERS"
	seedAuditQueue   = "MQS.SEED.AUDIT"
	seedBackoutQueue = "MQS.SEED.BACKOUT"
	seedSubQueue     = "MQS.SEED.SUBQ"
	seedTopic        = "MQS.SEED.EVENTS"
	seedTopicString  = "dev/seed/events"
	seedSubscription = "MQS.SEED.SUB"
	seedChannel      = "MQS.SEED.SDR"
	deadLetterQueue  = "DEV.DEAD.LETTER.QUEUE"
)

func requireIBMMQ(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.IBMMQ,
		Name:   "the ibm mq queue manager",
		Start:  "npm run e2e:ibmmq:up",
		Probe:  probeQueueManager,
	})
}

/*
 * probeQueueManager asks the REST API whether the queue manager is running.
 *
 * Not e2e.HTTPGet and not e2e.DialTCP, for two reasons that both matter.
 * The shared HTTP probe verifies certificates and the mqweb server presents
 * one it signed itself, so it would report every healthy environment as
 * absent. A TCP dial would go the other way and report an unhealthy one as
 * present: Liberty binds 9443 and serves the console while the queue manager
 * is still starting, which is the window this whole environment exists to
 * never hand a test.
 */
func probeQueueManager() error {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // a self-signed development certificate
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport}

	request, err := http.NewRequest(http.MethodGet, liveMQWeb+"/ibmmq/rest/v1/admin/qmgr/"+liveQueueManager, nil)
	if err != nil {
		return err
	}
	request.SetBasicAuth(liveAdminUser, liveAdminPass)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return &restError{Status: response.StatusCode, Message: http.StatusText(response.StatusCode)}
	}
	return nil
}

/*
 * liveProfile is the environment as a user would configure it.
 *
 * Skip-verify is on, and it is on in the profile rather than anywhere shared:
 * the mqweb server generated its own certificate, and the switch that accepts
 * it is a decision this profile makes. A test that turned verification off in
 * the driver, in the HTTP client or in an environment variable would be
 * turning it off for every user, which is the one thing this option must never
 * do - and TestLiveVerificationIsOffOnlyWhenTheProfileAsks below is what pins
 * that.
 */
func liveProfile() model.ConnectionProfile {
	return model.ConnectionProfile{
		ID:         1,
		Name:       "ibm mq e2e",
		Kind:       model.KindIBMMQ,
		Endpoints:  liveMQWeb,
		TimeoutSec: 20,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			OptionQueueManager:  liveQueueManager,
			OptionTLSSkipVerify: "true",
		},
		Secrets: map[string]string{
			SecretUsername:          liveAdminUser,
			SecretPassword:          liveAdminPass,
			SecretMessagingUsername: liveAppUser,
			SecretMessagingPassword: liveAppPass,
		},
	}
}

// adminOnlyProfile holds the administrative role and not the messaging one,
// which is what the developer image's admin account actually is.
func adminOnlyProfile() model.ConnectionProfile {
	profile := liveProfile()
	profile.Secrets = map[string]string{
		SecretUsername: liveAdminUser,
		SecretPassword: liveAdminPass,
		// Named explicitly rather than left to the fallback: the point of this
		// profile is a messaging credential that authenticates and holds the
		// wrong role, and an empty pair would reuse the administrative one and
		// arrive at the same place by accident.
		SecretMessagingUsername: liveAdminUser,
		SecretMessagingPassword: liveAdminPass,
	}
	return profile
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func liveConn(t *testing.T) *Conn {
	t.Helper()
	requireIBMMQ(t)
	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestLiveOpenReachesTheQueueManager(t *testing.T) {
	conn := liveConn(t)

	if conn.Kind() != model.KindIBMMQ {
		t.Errorf("kind = %q, want ibmmq", conn.Kind())
	}
	if conn.QueueManager() != liveQueueManager {
		t.Errorf("queue manager = %q, want %q", conn.QueueManager(), liveQueueManager)
	}
	if err := conn.Ping(liveContext(t)); err != nil {
		t.Errorf("ping: %v", err)
	}
}

/*
 * A profile that names no queue manager gets the one the server fronts.
 *
 * This is the ordinary case - most installations run one - and it is what
 * keeps the field optional on the form. It is also the half that has to be
 * proved against a real server: the listing is what supplies the name, and a
 * driver that defaulted to something would work here and address the wrong
 * queue manager on an installation with two.
 */
func TestLiveOpenDiscoversTheOnlyQueueManager(t *testing.T) {
	requireIBMMQ(t)
	profile := liveProfile()
	profile.Options[OptionQueueManager] = ""

	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if conn.QueueManager() != liveQueueManager {
		t.Errorf("discovered %q, want %q", conn.QueueManager(), liveQueueManager)
	}
}

// A queue manager that is not there fails at open, where the message can still
// name the field and list what the server does front. Discovering it at the
// first board instead would report every page as broken.
func TestLiveOpenRefusesAQueueManagerThatIsNotThere(t *testing.T) {
	requireIBMMQ(t)
	profile := liveProfile()
	profile.Options[OptionQueueManager] = "NOSUCHQM"

	conn, err := open(liveContext(t), profile)
	if err == nil {
		_ = conn.Close()
		t.Fatal("opened a connection to a queue manager that does not exist")
	}
	if !strings.Contains(err.Error(), liveQueueManager) {
		t.Errorf("error does not say which queue managers are there: %v", err)
	}
}

/*
 * The messaging interface is a tier of its own, and the probe is what settles
 * whether this connection has it.
 *
 * Both halves are asserted here because the environment can supply both, which
 * is unusual: the developer image ships one account per mqweb role, so the same
 * server answers "yes" to one credential and "the role is not mapped" to
 * another. A driver that assumed one credential reaches both interfaces would
 * pass every other test in this file.
 */
func TestLiveMessagingTierIsProbedSeparately(t *testing.T) {
	requireIBMMQ(t)

	t.Run("with the messaging account", func(t *testing.T) {
		conn, err := open(liveContext(t), liveProfile())
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		if reason := conn.probeMessaging(liveContext(t)); reason != "" {
			t.Errorf("messaging reported unavailable as %s, and the app account holds MQWebUser", reason)
		}
	})

	t.Run("with the administrative account only", func(t *testing.T) {
		conn, err := open(liveContext(t), adminOnlyProfile())
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		reason := conn.probeMessaging(liveContext(t))
		if reason != messagingForbidden {
			t.Errorf("messaging reason = %q, want %q; the admin account is not in MQWebMessaging",
				reason, messagingForbidden)
		}
	})

	t.Run("with a password the server will not take", func(t *testing.T) {
		profile := liveProfile()
		profile.Secrets[SecretMessagingPassword] = "not-the-password"

		conn, err := open(liveContext(t), profile)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		reason := conn.probeMessaging(liveContext(t))
		if reason != messagingRefused {
			t.Errorf("messaging reason = %q, want %q; a rejected credential and an unmapped "+
				"role send a reader to different places", reason, messagingRefused)
		}
	})
}

// An administrative credential the server will not take fails at open. It has
// to: every board reads through that interface, and a connection that opened
// anyway would report an empty queue manager rather than a refused login.
func TestLiveOpenRefusesAnAdministrativeCredentialTheServerRejects(t *testing.T) {
	requireIBMMQ(t)
	profile := liveProfile()
	profile.Secrets[SecretPassword] = "not-the-password"

	conn, err := open(liveContext(t), profile)
	if err == nil {
		_ = conn.Close()
		t.Fatal("opened a connection with a password the mqweb server rejects")
	}
}

/*
 * Verification is off only because this profile asked, and this is the test
 * that keeps it that way.
 *
 * The mqweb server signs its own certificate, so every test in this file needs
 * the switch - which is exactly the situation in which somebody quietly turns
 * verification off in the driver, or in a shared HTTP client, and nobody
 * notices for a release. The same profile with the switch off must fail, and
 * fail on the certificate rather than on anything else.
 */
func TestLiveVerificationIsOffOnlyWhenTheProfileAsks(t *testing.T) {
	requireIBMMQ(t)
	profile := liveProfile()
	profile.Options[OptionTLSSkipVerify] = "false"

	conn, err := open(liveContext(t), profile)
	if err == nil {
		_ = conn.Close()
		t.Fatal("connected to a self-signed mqweb server with verification on")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "certificate") {
		t.Errorf("failed for a reason other than the certificate: %v", err)
	}
}

// The queue manager's state is what Ping reads, and a closed connection stops
// answering. The registry closes on disconnect and on shutdown, so the second
// close has to be the one that does nothing.
func TestLiveCloseIsIdempotent(t *testing.T) {
	requireIBMMQ(t)
	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := conn.Ping(liveContext(t)); err == nil {
		t.Error("a closed connection still answers a ping")
	}
}

// find returns the destination named, or fails the test saying what was there.
func find(t *testing.T, destinations []*model.Destination, name string) *model.Destination {
	t.Helper()
	names := make([]string, 0, len(destinations))
	for _, destination := range destinations {
		if destination.Ref.Name == name {
			return destination
		}
		names = append(names, destination.Ref.Name)
	}
	t.Fatalf("no destination named %q; the listing held %s", name, strings.Join(names, ", "))
	return nil
}

/*
 * The listing crosses both interfaces, and that is what has to be proved
 * against a real queue manager: queues come from a REST resource and topics
 * from MQSC, and a driver that read only the first would produce a page that
 * looks complete and is missing half the topology.
 */
func TestLiveListDestinationsReadsQueuesAndTopics(t *testing.T) {
	conn := liveConn(t)

	destinations, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("ListDestinations: %v", err)
	}

	queue := find(t, destinations, seedQueue)
	if queue.Attribute(AttrKind) != KindQueue {
		t.Errorf("%s is a %q", seedQueue, queue.Attribute(AttrKind))
	}
	if queue.Attribute(AttrQueueType) != "local" {
		t.Errorf("%s is a %q queue", seedQueue, queue.Attribute(AttrQueueType))
	}
	if queue.Depth <= 0 {
		t.Errorf("%s reports depth %d; the seed put messages on it", seedQueue, queue.Depth)
	}
	// No partitions and no rates anywhere in this family, and the difference
	// between "none" and "zero" is the whole point of UnknownMetric: a zero
	// would read as an idle queue rather than as a figure MQ never reports.
	if queue.Partitions != model.UnknownMetric {
		t.Errorf("%s claims %d partitions, and IBM MQ divides nothing", seedQueue, queue.Partitions)
	}
	if queue.RateIn != model.UnknownMetric || queue.RateOut != model.UnknownMetric {
		t.Errorf("%s reports rates (%d in, %d out), and no MQSC command returns one",
			seedQueue, queue.RateIn, queue.RateOut)
	}

	topic := find(t, destinations, seedTopic)
	if topic.Attribute(AttrKind) != KindTopic {
		t.Errorf("%s is a %q", seedTopic, topic.Attribute(AttrKind))
	}
	// The object's name and the string publishers use are two different
	// things, and a page that showed only the first could not tell a reader
	// where to publish.
	if got := topic.Attribute(AttrTopicString); got != seedTopicString {
		t.Errorf("%s carries topic string %q, want %q", seedTopic, got, seedTopicString)
	}
	if topic.Subscribers < 1 {
		t.Errorf("%s reports %d subscribers; the seed made one", seedTopic, topic.Subscribers)
	}
	// A topic stores nothing: what a subscription is owed sits on the queue it
	// delivers to, which is a different row.
	if topic.Depth != model.UnknownMetric {
		t.Errorf("%s reports depth %d, and a topic holds nothing", seedTopic, topic.Depth)
	}
}

/*
 * SYSTEM.* is IBM's reserved prefix and the queue manager enforces it, so it
 * is a rule rather than a convention - but there are sixty of them on a fresh
 * queue manager and they would bury a listing.
 */
func TestLiveListDestinationsHidesTheQueueManagersOwn(t *testing.T) {
	conn := liveConn(t)

	visible, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("ListDestinations: %v", err)
	}
	for _, destination := range visible {
		if strings.HasPrefix(destination.Ref.Name, "SYSTEM.") {
			t.Errorf("%s is drawn by default, and the queue manager made it for itself",
				destination.Ref.Name)
		}
	}

	all, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{IncludeInternal: true})
	if err != nil {
		t.Fatalf("ListDestinations(internal): %v", err)
	}
	if len(all) <= len(visible) {
		t.Errorf("asking for internal objects returned %d rows against %d; "+
			"a fresh queue manager has some sixty of its own", len(all), len(visible))
	}
	// The image's own DEV.* objects are somebody's configuration rather than
	// the queue manager's, so they must not be filtered with them.
	find(t, visible, deadLetterQueue)
}

// The queue manager names one queue for what it cannot deliver, and a depth on
// that queue means something quite different from a depth anywhere else - so
// the row has to say which one it is.
func TestLiveTheQueueManagersDeadLetterQueueIsMarked(t *testing.T) {
	conn := liveConn(t)

	destinations, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("ListDestinations: %v", err)
	}

	marked := 0
	for _, destination := range destinations {
		if destination.Attribute(AttrDeadLetter) == "true" {
			marked++
			if destination.Ref.Name != deadLetterQueue {
				t.Errorf("%s is marked as the dead-letter queue, and the queue manager names %s",
					destination.Ref.Name, deadLetterQueue)
			}
		}
	}
	if marked != 1 {
		t.Errorf("%d queues are marked as the dead-letter queue; the queue manager names exactly one", marked)
	}
}

/*
 * A detail lookup has to work for both kinds, and a name alone does not say
 * which it is. A driver that asked the queue resource would report every topic
 * as missing, which is the mistake this pins.
 */
func TestLiveDestinationDetailAnswersForATopicToo(t *testing.T) {
	conn := liveConn(t)

	topic, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: seedTopic})
	if err != nil {
		t.Fatalf("DestinationDetail(%s): %v", seedTopic, err)
	}
	if topic.Attribute(AttrKind) != KindTopic {
		t.Errorf("%s came back as a %q", seedTopic, topic.Attribute(AttrKind))
	}

	if _, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: "MQS.TEST.NOTHING"}); err == nil {
		t.Error("a name nothing has came back as a destination")
	}
}

/*
 * A queue round trip through the REST resource, and a topic through MQSC.
 *
 * Both halves are here because they are two different code paths to the same
 * button: a driver that only ever created queues would pass a test that
 * created one, and the topic path would fail the first time somebody used it.
 */
func TestLiveCreateAndRemoveAQueueAndATopic(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	const queue = "MQS.TEST.CREATED.QUEUE"
	const topic = "MQS.TEST.CREATED.TOPIC"
	const topicString = "mq/studio/test/created"

	// Left behind by an interrupted run, which would otherwise fail the create.
	_ = conn.RemoveQueueGuarded(ctx, model.DestinationRef{Name: queue}, true, false)
	_ = conn.RemoveDestination(ctx, model.DestinationRef{Name: topic})

	if err := conn.CreateDestination(ctx, model.DestinationSpec{
		Ref: model.DestinationRef{Name: queue},
		Attributes: map[string]string{
			AttrKind:        KindQueue,
			AttrQueueType:   "local",
			AttrMaxDepth:    "1234",
			AttrDescription: "made by the live test",
		},
	}); err != nil {
		t.Fatalf("CreateDestination(queue): %v", err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveQueueGuarded(context.Background(), model.DestinationRef{Name: queue}, true, false)
	})

	made, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: queue})
	if err != nil {
		t.Fatalf("DestinationDetail(%s): %v", queue, err)
	}
	if made.Attribute(AttrMaxDepth) != "1234" {
		t.Errorf("%s has maximum depth %q, want the 1234 the create asked for",
			queue, made.Attribute(AttrMaxDepth))
	}
	if made.Attribute(AttrDescription) != "made by the live test" {
		t.Errorf("%s carries description %q", queue, made.Attribute(AttrDescription))
	}

	if err := conn.CreateDestination(ctx, model.DestinationSpec{
		Ref: model.DestinationRef{Name: topic},
		Attributes: map[string]string{
			AttrKind:        KindTopic,
			AttrTopicString: topicString,
		},
	}); err != nil {
		t.Fatalf("CreateDestination(topic): %v", err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveDestination(context.Background(), model.DestinationRef{Name: topic})
	})

	madeTopic, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: topic})
	if err != nil {
		t.Fatalf("DestinationDetail(%s): %v", topic, err)
	}
	if madeTopic.Attribute(AttrTopicString) != topicString {
		t.Errorf("%s carries topic string %q, want %q",
			topic, madeTopic.Attribute(AttrTopicString), topicString)
	}

	// The delete has to find each one through the right interface, which is
	// the half a name alone cannot decide.
	if err := conn.RemoveDestination(ctx, model.DestinationRef{Name: topic}); err != nil {
		t.Errorf("RemoveDestination(topic): %v", err)
	}
	if err := conn.RemoveDestination(ctx, model.DestinationRef{Name: queue}); err != nil {
		t.Errorf("RemoveDestination(queue): %v", err)
	}
	if _, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: queue}); err == nil {
		t.Errorf("%s is still there after being deleted", queue)
	}
}

// A topic without a string would be an object nobody publishes through, so it
// is refused here rather than created and then wondered about.
func TestLiveCreateRefusesATopicWithNoTopicString(t *testing.T) {
	conn := liveConn(t)

	err := conn.CreateDestination(liveContext(t), model.DestinationSpec{
		Ref:        model.DestinationRef{Name: "MQS.TEST.NOSTRING"},
		Attributes: map[string]string{AttrKind: KindTopic},
	})
	if err == nil {
		_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: "MQS.TEST.NOSTRING"})
		t.Fatal("created a topic object with no topic string")
	}
}

/*
 * A name the queue manager could never take is refused where the message can
 * still name the field.
 *
 * The command server's own refusal is a syntax error at a character position,
 * which reads as a broken driver rather than as a name that was never allowed.
 */
func TestLiveCreateRefusesANameIBMMQCannotHave(t *testing.T) {
	conn := liveConn(t)

	for _, name := range []string{"MQS TEST SPACES", strings.Repeat("Q", 49), "MQS-TEST-DASH"} {
		err := conn.CreateDestination(liveContext(t), model.DestinationSpec{
			Ref:        model.DestinationRef{Name: name},
			Attributes: map[string]string{AttrKind: KindQueue},
		})
		if err == nil {
			_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: name})
			t.Errorf("created a queue named %q", name)
		}
	}
}

/*
 * Deleting a queue that holds messages is refused, and the refusal is the
 * queue manager's own.
 *
 * That default is worth pinning: it is the difference between a delete that
 * discards a backlog nobody meant to lose and one that stops and asks. The
 * purge path is the caller saying it knows, and it is the only thing in this
 * driver that throws data away.
 */
func TestLiveDeletingAQueueWithMessagesNeedsAsking(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	const queue = "MQS.TEST.HOLDING"
	_ = conn.RemoveQueueGuarded(ctx, model.DestinationRef{Name: queue}, true, false)
	if err := conn.CreateDestination(ctx, model.DestinationSpec{
		Ref:        model.DestinationRef{Name: queue},
		Attributes: map[string]string{AttrKind: KindQueue, AttrQueueType: "local"},
	}); err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveQueueGuarded(context.Background(), model.DestinationRef{Name: queue}, true, false)
	})

	// Put one message on it through the same interface the send console uses,
	// so the depth is real rather than simulated.
	if err := c8put(t, conn, queue, "holding"); err != nil {
		t.Fatalf("putting a message on %s: %v", queue, err)
	}

	if err := conn.RemoveDestination(ctx, model.DestinationRef{Name: queue}); err == nil {
		t.Fatal("deleted a queue holding a message without being asked to discard it")
	}
	if err := conn.RemoveQueueGuarded(ctx, model.DestinationRef{Name: queue}, true, false); err != nil {
		t.Fatalf("RemoveQueueGuarded(purge): %v", err)
	}
	if _, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: queue}); err == nil {
		t.Error("the queue survived a purging delete")
	}
}

// c8put writes one message through the messaging interface, which is what the
// live tests use whenever a queue needs a depth they did not fake.
func c8put(t *testing.T, conn *Conn, queue, body string) error {
	t.Helper()
	_, err := conn.rest.messagingPost(
		liveContext(t),
		"/qmgr/"+conn.qmgr+"/queue/"+queue+"/message",
		"text/plain;charset=utf-8", []byte(body), nil)
	return err
}

/*
 * The channels page, and the reason it is not the clients page.
 *
 * Every channel the queue manager has is a definition somebody made: it is
 * listed whether or not anything is using it, and the SVRCONN definitions -
 * how every client application reaches the queue manager at all - are the ones
 * the REST channel resource does not return, which is why this driver reads
 * MQSC instead. A listing that missed them would be missing the row anybody
 * opens the page for.
 */
func TestLiveListChannelsIncludesTheOnesClientsArriveOn(t *testing.T) {
	conn := liveConn(t)

	channels, err := conn.ListChannels(liveContext(t))
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) == 0 {
		t.Fatal("no channels at all; a fresh queue manager defines a dozen")
	}

	byName := make(map[string]*model.Channel, len(channels))
	types := make(map[model.ChannelType]int, 8)
	for _, channel := range channels {
		byName[channel.Name] = channel
		types[channel.Type]++
	}

	// The developer image's own, and the type the REST resource omits.
	client := byName["DEV.APP.SVRCONN"]
	if client == nil {
		t.Fatal("DEV.APP.SVRCONN is missing; the REST channel resource returns no " +
			"server-connection channel, which is why this reads MQSC")
	}
	if client.Type != model.ChannelServerConnection {
		t.Errorf("DEV.APP.SVRCONN is a %q", client.Type)
	}
	if types[model.ChannelServerConnection] == 0 || types[model.ChannelSender] == 0 {
		t.Errorf("the listing holds %v; both a client-facing and a message channel are defined",
			types)
	}
}

/*
 * A channel that is not running is what the page exists to show, and a fresh
 * queue manager cannot produce one: every channel it ships is defined and has
 * never been started. The seed starts one against an unroutable address, which
 * is the state an operator actually goes looking for.
 */
func TestLiveAChannelThatCannotConnectSaysSo(t *testing.T) {
	conn := liveConn(t)

	channels, err := conn.ListChannels(liveContext(t))
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}

	var started *model.Channel
	for _, channel := range channels {
		if channel.Name == seedChannel {
			started = channel
		}
	}
	if started == nil {
		e2e.Missing(t, "%s is not defined; run: npm run e2e:ibmmq:seed", seedChannel)
		return
	}

	if started.Status == "" {
		t.Fatalf("%s reports no status at all; the seed started it", seedChannel)
	}
	if started.Status == model.ChannelRunning {
		t.Errorf("%s is running, and it points at an address in TEST-NET-1", seedChannel)
	}
	if started.Instances < 1 {
		t.Errorf("%s reports %d instances while carrying a status", seedChannel, started.Instances)
	}
	// The configured address survives onto the row: without it a reader cannot
	// tell a channel that is retrying from one that was never given a partner.
	if !strings.Contains(started.ConnectionName, "192.0.2.10") {
		t.Errorf("%s reports connection name %q, want the seed's unroutable address",
			seedChannel, started.ConnectionName)
	}
	if started.TransmissionQueue == "" {
		t.Errorf("%s names no transmission queue, and a sender without one is misconfigured",
			seedChannel)
	}
}

/*
 * A definition with nothing running reports no status, and that is not the
 * same as stopped.
 *
 * It is the distinction the whole page turns on: "inactive" is the normal
 * state of a channel nobody uses, and the same channel reporting "retrying"
 * means something is wrong. A driver that defaulted the empty case to a word
 * would erase the difference.
 */
func TestLiveAChannelNobodyHasStartedReportsNoStatus(t *testing.T) {
	conn := liveConn(t)

	channels, err := conn.ListChannels(liveContext(t))
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}

	for _, channel := range channels {
		if channel.Name != "SYSTEM.DEF.SENDER" {
			continue
		}
		if channel.Status != "" {
			t.Errorf("SYSTEM.DEF.SENDER reports status %q, and nothing has ever started it",
				channel.Status)
		}
		if channel.Instances != 0 {
			t.Errorf("SYSTEM.DEF.SENDER reports %d instances", channel.Instances)
		}
		// A count of zero would read as "has carried nothing", which is a
		// different statement from "has never run".
		if channel.Messages != model.UnknownMetric {
			t.Errorf("SYSTEM.DEF.SENDER reports %d messages rather than none at all",
				channel.Messages)
		}
		return
	}
	t.Error("SYSTEM.DEF.SENDER is missing; every queue manager defines it")
}

// depthOf reads one queue's depth straight from the admin interface, which is
// what the non-destructive claim below is measured against.
func depthOf(t *testing.T, conn *Conn, queue string) int64 {
	t.Helper()
	destination, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: queue})
	if err != nil {
		t.Fatalf("DestinationDetail(%s): %v", queue, err)
	}
	return destination.Depth
}

/*
 * Browsing takes nothing, and this is the test that says so in figures.
 *
 * It is worth measuring rather than asserting because the same interface has
 * the other operation: DELETE on the message resource consumes, and a driver
 * that reached for it would pass every other test in this file while quietly
 * draining whatever anybody looked at.
 */
func TestLiveBrowsingLeavesTheQueueAlone(t *testing.T) {
	conn := liveConn(t)

	before := depthOf(t, conn, seedQueue)
	if before <= 0 {
		e2e.Missing(t, "%s is empty; run: npm run e2e:ibmmq:seed", seedQueue)
		return
	}

	messages, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{Topic: seedQueue})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	if len(messages) == 0 {
		t.Fatalf("%s holds %d messages and the browse returned none", seedQueue, before)
	}

	// Twice, because a single read could consume and still look right.
	if _, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{Topic: seedQueue}); err != nil {
		t.Fatalf("second QueryMessages: %v", err)
	}
	if after := depthOf(t, conn, seedQueue); after != before {
		t.Errorf("%s held %d messages and holds %d after two browses; the browse is consuming",
			seedQueue, before, after)
	}

	first := messages[0]
	if first.Body == "" {
		t.Error("the first message came back with no body")
	}
	if first.MessageID == "" {
		t.Error("the first message came back with no identifier")
	}
	if first.Topic != seedQueue {
		t.Errorf("the first message names %q as its destination", first.Topic)
	}
	// No put time anywhere in the descriptor, and the driver must not invent
	// one from the clock on this machine.
	if first.StoreTime != "" || first.StoreTimestamp != 0 {
		t.Errorf("a message carries a store time (%q / %d), and mqweb returns none",
			first.StoreTime, first.StoreTimestamp)
	}
}

// The limit is a real limit rather than a page size: each message costs a
// request of its own, so a browse that ignored it would be fifty round trips
// where the caller asked for three.
func TestLiveBrowsingHonoursTheLimit(t *testing.T) {
	conn := liveConn(t)

	messages, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic:      seedQueue,
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	if len(messages) > 3 {
		t.Errorf("asked for 3 messages and got %d", len(messages))
	}
}

/*
 * A message whose body the server will not decode is listed rather than
 * dropped, and this is the case that proves it.
 *
 * Every dead letter carries a dead-letter header in front of its payload, so
 * mqweb answers 501 for all of them. Which messages are on the queue is worth
 * knowing even when their contents cannot be shown, and a browse that returned
 * fewer rows than the depth would be the more confusing answer.
 */
func TestLiveAMessageTheServerWillNotDecodeIsStillListed(t *testing.T) {
	conn := liveConn(t)

	depth := depthOf(t, conn, deadLetterQueue)
	if depth <= 0 {
		e2e.Missing(t, "%s is empty; run: npm run e2e:ibmmq:seed", deadLetterQueue)
		return
	}

	messages, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{Topic: deadLetterQueue})
	if err != nil {
		t.Fatalf("QueryMessages(%s): %v", deadLetterQueue, err)
	}
	if int64(len(messages)) != depth {
		t.Errorf("%s holds %d messages and the browse returned %d rows",
			deadLetterQueue, depth, len(messages))
	}

	for _, message := range messages {
		format, refused := message.Properties[PropBodyUnavailable]
		if !refused {
			t.Errorf("%s came back with a body, and every dead letter carries a dead-letter header",
				message.MessageID)
			continue
		}
		if format != "MQDEAD" {
			t.Errorf("%s was refused as format %q, want MQDEAD", message.MessageID, format)
		}
		if message.Body != "" {
			t.Errorf("%s is marked unreadable and carries a body anyway", message.MessageID)
		}
	}
}

// One message by its identifier, which is the other half of the message page:
// the browse hands out identifiers and this is what opens one.
func TestLiveMessageByIDFindsWhatTheBrowseListed(t *testing.T) {
	conn := liveConn(t)

	messages, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic:      seedQueue,
		MaxResults: 1,
	})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	if len(messages) == 0 {
		e2e.Missing(t, "%s is empty; run: npm run e2e:ibmmq:seed", seedQueue)
		return
	}

	wanted := messages[0]
	found, err := conn.MessageByID(liveContext(t), seedQueue, wanted.MessageID)
	if err != nil {
		t.Fatalf("MessageByID: %v", err)
	}
	if found == nil {
		t.Fatalf("%s was listed by the browse and not found by its id", wanted.MessageID)
	}
	if found.Body != wanted.Body {
		t.Errorf("the same message read twice has two bodies: %q and %q", wanted.Body, found.Body)
	}

	// An identifier nothing has is nil rather than an error: a message taken by
	// a consumer between two clicks is gone, not a failure. The queue manager
	// answers that with 204 rather than 404, which is a success with an empty
	// body and would otherwise read as a message with no content.
	missing, err := conn.MessageByID(liveContext(t), seedQueue, "ff"+strings.Repeat("0", 46))
	if err != nil {
		t.Errorf("looking for an identifier nothing has failed: %v", err)
	}
	if missing != nil {
		t.Error("an identifier nothing has came back as a message")
	}
}

/*
 * The all-zero identifier is refused, and it has to be.
 *
 * Twenty-four zero bytes is MQMI_NONE, which the queue manager reads as "no
 * selector" rather than as an identifier nothing has - so a browse sent one
 * comes back with whatever is at the front of the queue. A user who pasted a
 * blank id, or a caller reusing a zeroed field, would be handed a real message
 * and told it was the one they asked for.
 */
func TestLiveTheEmptyMessageIdentifierIsRefusedRatherThanMatched(t *testing.T) {
	conn := liveConn(t)

	if _, err := conn.MessageByID(liveContext(t), seedQueue, strings.Repeat("0", 48)); err == nil {
		t.Fatal("the empty message id was accepted; the queue manager answers it with the " +
			"first message on the queue")
	}
}

/*
 * A send, read back through the browse that the same interface answers.
 *
 * The round trip is the point: the descriptor goes out in request headers and
 * comes back in response headers, and every one of those pairs is a place a
 * name could be wrong in a way nothing else notices.
 */
func TestLiveSendReachesTheQueueWithItsDescriptor(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	const queue = "MQS.TEST.SENT"
	_ = conn.RemoveQueueGuarded(ctx, model.DestinationRef{Name: queue}, true, false)
	if err := conn.CreateDestination(ctx, model.DestinationSpec{
		Ref:        model.DestinationRef{Name: queue},
		Attributes: map[string]string{AttrKind: KindQueue, AttrQueueType: "local"},
	}); err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveQueueGuarded(context.Background(), model.DestinationRef{Name: queue}, true, false)
	})

	correlation := strings.Repeat("ab", 24)
	result, err := conn.Publish(ctx, PublishRequest{
		Queue:         queue,
		Body:          `{"sent":"by the live test"}`,
		ContentType:   "application/json",
		CorrelationID: correlation,
		Persistent:    true,
		ExpirySeconds: 600,
		Properties:    map[string]string{"orderNo": "42"},
		Count:         3,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.Sent != 3 {
		t.Errorf("sent %d of 3", result.Sent)
	}
	if result.MessageID == "" {
		t.Error("the send reported no message id, and the queue manager assigns one to every message")
	}

	if depth := depthOf(t, conn, queue); depth != 3 {
		t.Fatalf("%s holds %d messages after sending three", queue, depth)
	}

	messages, err := conn.QueryMessages(ctx, model.MessageQueryParams{Topic: queue})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("browsed %d of the three messages sent", len(messages))
	}

	first := messages[0]
	if first.Body != `{"sent":"by the live test"}` {
		t.Errorf("the body came back as %q", first.Body)
	}
	if got := first.Properties[PropCorrelationID]; !strings.EqualFold(got, correlation) {
		t.Errorf("correlation id came back as %q, want %q", got, correlation)
	}
	if got := first.Properties[PropPersistence]; got != "persistent" {
		t.Errorf("persistence came back as %q; the send asked for persistent", got)
	}
	// Expiry is set in tenths of a second, which is the sort of thing that is
	// quietly wrong by a factor of ten. Anything but "unlimited" proves the
	// header reached the descriptor.
	if got := first.Properties[PropExpiry]; got == "" || got == "unlimited" {
		t.Errorf("expiry came back as %q; the send asked for 600 seconds", got)
	}
}

/*
 * Sending to a topic is refused here rather than at the queue manager.
 *
 * The messaging interface has no topic resource at all, so a send that reached
 * it would fail with a 404 naming a URI. What the reader needs to know is that
 * publishing needs an MQ client, which is a statement about the interface
 * rather than about the name they typed.
 */
func TestLiveSendRefusesATopic(t *testing.T) {
	conn := liveConn(t)

	_, err := conn.Publish(liveContext(t), PublishRequest{Queue: seedTopic, Body: "nope"})
	if err == nil {
		t.Fatal("sent a message to a topic object")
	}
	if !strings.Contains(err.Error(), "topic") {
		t.Errorf("the refusal does not say the destination is a topic: %v", err)
	}
}

/*
 * The canonical send refuses the three arguments this family has no
 * counterpart for, rather than dropping them.
 *
 * A tag and a key are RocketMQ's, and a delay level is a scheduled send MQ has
 * no equivalent of. Accepting them would be three controls that appear to work.
 */
func TestLiveTheCanonicalSendRefusesWhatItCannotCarry(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	if _, err := conn.SendMessage(ctx, seedQueue, "orders", "", "body", 0); err == nil {
		t.Error("a tag was accepted, and an ibm mq message has none")
	}
	if _, err := conn.SendMessage(ctx, seedQueue, "", "key-1", "body", 0); err == nil {
		t.Error("a key was accepted, and an ibm mq message has none")
	}
	if _, err := conn.SendMessage(ctx, seedQueue, "", "", "body", 3); err == nil {
		t.Error("a delay level was accepted, and ibm mq has no scheduled send")
	}
}

// A correlation identifier that is not 24 bytes of hexadecimal is refused
// where the message can name the field, rather than by the server naming a
// hex string.
func TestLiveSendRefusesAMalformedCorrelationID(t *testing.T) {
	conn := liveConn(t)

	for _, correlation := range []string{"not-hex", strings.Repeat("a", 47), strings.Repeat("z", 48)} {
		_, err := conn.Publish(liveContext(t), PublishRequest{
			Queue:         seedQueue,
			Body:          "body",
			CorrelationID: correlation,
		})
		if err == nil {
			t.Errorf("accepted %q as a correlation id", correlation)
		}
	}
}

/*
 * Dead letters, found by walking the configuration backwards.
 *
 * Both pointers are asserted because they are two different mechanisms that
 * happen to end at the same page: the queue manager fills its own DEADQ, and a
 * backout queue is filled by whichever application decided to give up. A
 * driver that found only the first would report a queue manager as having one
 * dead-letter queue when it has three.
 */
func TestLiveDeadLetterQueuesFollowBothPointers(t *testing.T) {
	conn := liveConn(t)

	queues, err := conn.DeadLetterQueues(liveContext(t), "")
	if err != nil {
		t.Fatalf("DeadLetterQueues: %v", err)
	}
	if len(queues) == 0 {
		t.Fatal("no dead-letter queues at all; the queue manager names one for itself")
	}

	byName := make(map[string]*model.DeadLetterQueue, len(queues))
	for _, queue := range queues {
		byName[queue.Name] = queue
	}

	// The queue manager's own, whose source is the queue manager rather than a
	// queue - there is nothing else to name in its place.
	managers := byName[deadLetterQueue]
	if managers == nil {
		t.Fatalf("%s is not listed, and the queue manager's DEADQ names it", deadLetterQueue)
	}
	if managers.Depth <= 0 {
		e2e.Missing(t, "%s is empty; run: npm run e2e:ibmmq:seed", deadLetterQueue)
		return
	}
	if len(managers.Sources) != 1 {
		t.Fatalf("%s has %d sources; only the queue manager fills it",
			deadLetterQueue, len(managers.Sources))
	}
	if managers.Sources[0].Queue != liveQueueManager {
		t.Errorf("%s names %q as its source, want the queue manager",
			deadLetterQueue, managers.Sources[0].Queue)
	}
	if managers.Sources[0].Exchange != attributeDEADQ {
		t.Errorf("%s is attributed to %q, want DEADQ; a backout queue and the queue "+
			"manager's own are filled by different things",
			deadLetterQueue, managers.Sources[0].Exchange)
	}

	// The seed's backout queue, which nothing marks: it is a dead-letter queue
	// only because MQS.SEED.AUDIT points at it.
	backout := byName[seedBackoutQueue]
	if backout == nil {
		e2e.Missing(t, "%s is not listed; run: npm run e2e:ibmmq:seed", seedBackoutQueue)
		return
	}
	if len(backout.Sources) != 1 || backout.Sources[0].Queue != seedAuditQueue {
		t.Fatalf("%s reports sources %+v; the seed points %s at it",
			seedBackoutQueue, backout.Sources, seedAuditQueue)
	}
	if backout.Sources[0].Exchange != attributeBOQNAME {
		t.Errorf("%s is attributed to %q, want BOQNAME", seedBackoutQueue, backout.Sources[0].Exchange)
	}
	// The threshold decides whether the pointer is ever travelled at all: a
	// backout queue with a threshold of zero receives nothing.
	if backout.Sources[0].RoutingKey != "3" {
		t.Errorf("%s reports a backout threshold of %q, and the seed set 3",
			seedBackoutQueue, backout.Sources[0].RoutingKey)
	}
}

// A queue nothing points at is not a dead-letter queue, however it is named.
// The page is built by inverting configuration, so a name that merely looks
// like one must not appear.
func TestLiveAQueueNothingPointsAtIsNotADeadLetterQueue(t *testing.T) {
	conn := liveConn(t)

	queues, err := conn.DeadLetterQueues(liveContext(t), "")
	if err != nil {
		t.Fatalf("DeadLetterQueues: %v", err)
	}
	for _, queue := range queues {
		if queue.Name == seedQueue {
			t.Errorf("%s is listed as a dead-letter queue, and nothing points at it", seedQueue)
		}
		if len(queue.Sources) == 0 {
			t.Errorf("%s is listed with no source, which is what makes a queue one at all",
				queue.Name)
		}
	}
}
