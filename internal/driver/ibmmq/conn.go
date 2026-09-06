package ibmmq

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amigoer/mq-studio/internal/model"
)

var errConnectionDown = errors.New("ibm mq connection is not open")

// The reasons a capability can be missing here, as i18n keys rather than
// sentences. The renderer turns them into the user's language; an English
// frame around one would put the key itself on screen.
//
// They are split finer than "the messaging api is unavailable" for the reason
// ActiveMQ splits its AMQP reasons and MQTT splits its $SYS ones: "the server
// did not accept this credential" sends a user to the connection form, "the
// credential is fine and the role is not mapped" sends them to the mqweb
// server's configuration, and one sentence covering both sends half of them
// to the wrong place.
const (
	messagingForbidden = "mq.ibmmq.degraded.messagingForbidden"
	messagingRefused   = "mq.ibmmq.degraded.messagingRefused"
)

// Caveats, which are a different thing from a degraded reason: the capability
// works, and doing it has a consequence worth saying out loud.
const (
	// browseCharacterOnly is the one consequence of browsing here, and it is
	// not the one every other hosted family carries.
	//
	// A browse takes nothing: the message list and the message read are both
	// non-destructive, the queue's depth is the same afterwards, and any
	// number of readers can look at the same message. What the mqweb server
	// will not do is hand back a body it cannot decode as text - a dead
	// letter, a PCF event, an application's own structure - so those messages
	// are listed with their identifier and format and refused when opened.
	browseCharacterOnly = "mq.ibmmq.caveat.browseCharacterOnly"

	// sendQueueOnly is the matching limit on the way in. The messaging
	// interface has no topic resource at all, and it refuses a body that is
	// not character data outright.
	sendQueueOnly = "mq.ibmmq.caveat.sendQueueOnly"
)

// defaultTimeout is what a profile that named none gets. The mqweb server is
// a Liberty application in front of a queue manager, and a cold one can take
// a second or two to answer its first call.
const defaultTimeout = 10 * time.Second

// tiers is what answered when the connection opened.
type tiers struct {
	// messagingReason is empty when the messaging interface is usable, and
	// otherwise the i18n key saying why it is not.
	messagingReason string
}

// Conn is one live connection to one queue manager through one mqweb server.
//
// "One connection" is an HTTP client rather than a socket: every call is a
// request that stands alone, so there is nothing held open between them and
// nothing to reconnect. What is held is which queue manager the profile
// settled on, because every path this driver builds names it.
type Conn struct {
	rest   *restClient
	config clientConfig
	qmgr   string

	capabilities model.Capabilities
	closeOnce    sync.Once
	closed       chan struct{}
}

// clientConfig is the profile reduced to what this driver actually dials.
type clientConfig struct {
	mqweb      string
	qmgr       string
	admin      credential
	messaging  credential
	timeout    time.Duration
	skipVerify bool
}

// Kind identifies the family.
func (c *Conn) Kind() model.MQKind { return model.KindIBMMQ }

// Capabilities is what this endpoint can do.
func (c *Conn) Capabilities() model.Capabilities { return c.capabilities }

// QueueManager is which queue manager this connection is pointed at. Every
// path the driver builds names it, and the boards print it.
func (c *Conn) QueueManager() string { return c.qmgr }

// Ping asks the queue manager for its own state.
//
// Not the mqweb server's home page, and the distinction matters more here
// than it does elsewhere: Liberty binds 9443 and serves the console well
// before the queue manager has started and again after it has stopped, so an
// HTTP check on the server reports a healthy endpoint in both windows. This
// reads the queue manager's state field, which only the queue manager can
// fill.
func (c *Conn) Ping(ctx context.Context) error {
	if err := c.live(); err != nil {
		return err
	}
	state, err := c.queueManagerState(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(state, "running") {
		return fmt.Errorf("queue manager %s is %s", c.qmgr, state)
	}
	return nil
}

// Close releases what the connection holds. The registry closes on disconnect
// and on shutdown, so the second call has to be the one that does nothing.
func (c *Conn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

// live reports whether the connection is still usable.
func (c *Conn) live() error {
	if c.rest == nil {
		return errConnectionDown
	}
	select {
	case <-c.closed:
		return errConnectionDown
	default:
		return nil
	}
}

// capabilities is the family's best case.
//
// It grows one port at a time: CheckConformance fails a capability with no
// interface behind it, so each one arrives in the commit that implements it
// rather than as a promise the connection cannot keep.
func capabilities() []model.Capability {
	return []model.Capability{
		model.CapDestinationList,
		model.CapDestinationCreate,
		model.CapDestinationDelete,
	}
}

// open dials the mqweb server, settles which queue manager was meant, and
// probes the messaging tier.
func open(ctx context.Context, profile model.ConnectionProfile) (*Conn, error) {
	config, err := configOf(profile)
	if err != nil {
		return nil, err
	}

	client, err := newRESTClient(config.mqweb, config.admin, config.messaging, config.timeout, config.skipVerify)
	if err != nil {
		return nil, err
	}

	conn := &Conn{rest: client, config: config, closed: make(chan struct{})}
	qmgr, err := conn.resolveQueueManager(ctx)
	if err != nil {
		return nil, err
	}
	conn.qmgr = qmgr

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("%s at %s did not answer: %w", qmgr, config.mqweb, err)
	}
	conn.capabilities = conn.declare(conn.probeMessaging(ctx))
	return conn, nil
}

/*
 * declare turns what answered into the capability set the pages gate on.
 *
 * Two things vary. The messaging interface is a tier of its own - it is
 * authorised separately from the administrative one, and a credential that
 * holds only the administrative role reaches every board except the two that
 * touch messages - so its capabilities are degraded with a reason rather than
 * dropped, which is the difference between a page that explains itself and a
 * page that is simply missing.
 *
 * The caveats do not vary, because what they describe is the interface rather
 * than the deployment: the messaging interface carries character data and
 * nothing else, in both directions, on every queue manager there is.
 */
func (c *Conn) declare(messagingReason string) model.Capabilities {
	declared := model.Capabilities{
		Supported: capabilities(),
		Degraded:  map[model.Capability]string{},
		Caveats:   map[model.Capability]string{},
	}
	if messagingReason != "" {
		for _, capability := range messagingCapabilities() {
			declared.Supported = without(declared.Supported, capability)
			declared.Degraded[capability] = messagingReason
		}
	}
	return declared
}

// messagingCapabilities are the ones the second interface answers, and the
// ones that go degraded together when it will not take this credential.
func messagingCapabilities() []model.Capability {
	return []model.Capability{}
}

func without(capabilities []model.Capability, unwanted model.Capability) []model.Capability {
	kept := make([]model.Capability, 0, len(capabilities))
	for _, capability := range capabilities {
		if capability != unwanted {
			kept = append(kept, capability)
		}
	}
	return kept
}

/*
 * probeMessaging reports why the messaging interface is unusable, or "" when
 * it is usable.
 *
 * The probe asks for the message list of a queue that does not exist, and
 * reads the refusal rather than the answer. That is deliberate: a name that
 * does exist would have to be guessed, and the two outcomes worth telling
 * apart both arrive before any queue is looked at. A credential the server
 * will not authenticate answers 401; one it authenticates and has not mapped
 * to the MQWebUser role answers 403 with MQWB0108E. Anything else - including
 * the 404 that says the queue is not there, which is the success case here -
 * means the interface is open to this credential.
 */
func (c *Conn) probeMessaging(ctx context.Context) string {
	_, _, err := c.rest.messagingGet(ctx, "/qmgr/"+c.qmgr+"/queue/"+probeQueueName+"/messagelist?limit=1")
	switch {
	case err == nil:
		return ""
	case refused(err):
		return messagingRefused
	case roleMissing(err):
		return messagingForbidden
	default:
		return ""
	}
}

// probeQueueName is a name no queue manager has. It is only ever used to
// provoke a refusal, so it must not be creatable by accident: MQ object names
// are at most 48 characters and this is one of them, chosen to read as what
// it is if it ever reaches a log.
const probeQueueName = "MQ.STUDIO.PROBE.NO.SUCH.QUEUE"

/*
 * resolveQueueManager settles which queue manager this profile meant.
 *
 * One mqweb server can front several. There is no rule saying which a profile
 * meant, so several with none named is a configuration the user resolves in
 * the form rather than one this picks for them - the same call ActiveMQ makes
 * when one JVM registers more than one broker. A name that was given is taken
 * as given and checked, so a typo fails here, where the message can name the
 * field, rather than at the first board that asks for a queue.
 */
func (c *Conn) resolveQueueManager(ctx context.Context) (string, error) {
	var listing struct {
		QMgr []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"qmgr"`
	}
	if err := c.rest.adminGet(ctx, "/qmgr", &listing); err != nil {
		return "", fmt.Errorf("no mqweb server answered at %s: %w", c.config.mqweb, err)
	}

	names := make([]string, 0, len(listing.QMgr))
	for _, entry := range listing.QMgr {
		if name := strings.TrimSpace(entry.Name); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	if wanted := c.config.qmgr; wanted != "" {
		for _, name := range names {
			if strings.EqualFold(name, wanted) {
				return name, nil
			}
		}
		return "", fmt.Errorf("%s has no queue manager named %q; it has %s",
			c.config.mqweb, wanted, strings.Join(names, ", "))
	}

	switch len(names) {
	case 0:
		return "", fmt.Errorf("%s has no queue manager", c.config.mqweb)
	case 1:
		return names[0], nil
	default:
		return "", fmt.Errorf("%s fronts %s; name the one this connection is for",
			c.config.mqweb, strings.Join(names, ", "))
	}
}

// queueManagerState reads the one field that says the queue manager is up.
func (c *Conn) queueManagerState(ctx context.Context) (string, error) {
	var listing struct {
		QMgr []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"qmgr"`
	}
	if err := c.rest.adminGet(ctx, "/qmgr/"+c.qmgr, &listing); err != nil {
		return "", err
	}
	if len(listing.QMgr) == 0 {
		return "", fmt.Errorf("%s no longer has a queue manager named %s", c.config.mqweb, c.qmgr)
	}
	return listing.QMgr[0].State, nil
}

// configOf reduces a profile to what this driver dials.
func configOf(profile model.ConnectionProfile) (clientConfig, error) {
	mqweb := firstEndpoint(profile.Endpoints)
	if mqweb == "" {
		return clientConfig{}, errors.New("no mqweb address configured")
	}

	timeout := time.Duration(profile.TimeoutSec) * time.Second
	if profile.TimeoutSec <= 0 {
		timeout = defaultTimeout
	}

	config := clientConfig{
		mqweb:      mqweb,
		qmgr:       strings.TrimSpace(profile.Option(OptionQueueManager)),
		timeout:    timeout,
		skipVerify: isTrue(profile.Option(OptionTLSSkipVerify)),
	}

	if profile.Auth.Mechanism != model.AuthNone {
		config.admin = credential{
			username: profile.Secret(SecretUsername),
			password: profile.Secret(SecretPassword),
		}
	}

	// The administrative credential is the fallback, which is the common case:
	// a deployment that maps both roles to one group has one account for both
	// interfaces. The developer image is the case this exists for - it ships
	// one user per role, and neither can do the other's work.
	config.messaging = credential{
		username: profile.Secret(SecretMessagingUsername),
		password: profile.Secret(SecretMessagingPassword),
	}
	if config.messaging.empty() {
		config.messaging = config.admin
	}
	return config, nil
}

// firstEndpoint takes the mqweb address out of the profile's list.
//
// The field is a list because every family's is, not because a second address
// would mean anything: two mqweb servers are two installations, and this
// driver reads one queue manager through one of them.
func firstEndpoint(endpoints string) string {
	for _, part := range strings.Split(endpoints, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			return withScheme(trimmed)
		}
	}
	return ""
}

// withScheme accepts the host:port a user types out of habit, because every
// other family's endpoint field takes one and the muscle memory is real. The
// default is https rather than http: the mqweb server is TLS-only unless it
// has been reconfigured, which is why the skip-verify switch exists at all.
func withScheme(endpoint string) string {
	if strings.Contains(endpoint, "://") {
		return endpoint
	}
	return "https://" + endpoint
}

func isTrue(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}
