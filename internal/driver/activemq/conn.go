package activemq

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amigoer/mq-studio/internal/model"
)

// The reasons a capability can be missing here, as i18n keys rather than
// sentences. The renderer turns them into the user's language; an English
// frame around them would put the key itself on screen.
//
// They are split finer than "AMQP is unavailable" on purpose, the same way
// MQTT splits sysRefused from sysSilent: "you did not configure it" sends a
// user to the connection form, "the broker refused" sends them to the broker's
// acceptor list, and one sentence covering both sends half of them to the
// wrong place.
const (
	amqpAbsent      = "mq.activemq.degraded.amqpAbsent"
	amqpUnreachable = "mq.activemq.degraded.amqpUnreachable"
	amqpForbidden   = "mq.activemq.degraded.amqpForbidden"
)

// Caveats, which are a different thing from a degraded reason: the capability
// works, and has a consequence worth saying out loud.
const (
	browseCapped = "mq.activemq.caveat.browseCapped"
)

// classicBrowseCap is how many messages Classic's browse() will return.
//
// It is the maxBrowsePageSize destination policy, which defaults to 400. The
// attribute is not readable over JMX - reading MaxBrowsePageSize off the
// destination MBean answers 404 - so the driver cannot discover the deployment's
// value and instead reports the cap when a browse comes back exactly this long
// while the queue is deeper. Confirmed against 6.2.0: a queue holding 500
// browses 400.
const classicBrowseCap = 400

var errConnectionDown = errors.New("activemq connection is not open")

// tiers is what answered when the connection opened.
type tiers struct {
	// product is which broker this is. Never empty on an open connection:
	// a console that answers for neither domain is not an ActiveMQ endpoint,
	// and Open fails rather than returning a connection with no tree to read.
	product product

	// amqpReason is empty when the AMQP tier is live, and otherwise the i18n
	// key saying why it is not.
	amqpReason string
}

// Conn is one live connection to one broker.
type Conn struct {
	jolokia *jolokiaClient
	names   names
	tiers   tiers
	config  clientConfig

	capabilities model.Capabilities
	closeOnce    sync.Once
	closed       chan struct{}
}

// clientConfig is the profile reduced to what this driver actually dials.
type clientConfig struct {
	console    string
	agentPath  string
	brokerName string
	origin     string
	username   string
	password   string
	amqpURL    string
	amqpUser   string
	amqpPass   string
	timeout    time.Duration
	skipVerify bool
}

// Kind identifies the family.
func (c *Conn) Kind() model.MQKind { return model.KindActiveMQ }

// Capabilities is what this endpoint can do.
func (c *Conn) Capabilities() model.Capabilities { return c.capabilities }

// Ping reads the broker's own MBean rather than fetching the console page.
//
// The distinction matters here more than it does elsewhere: Jetty binds the
// console port well before the broker has started, and again after the broker
// has stopped, so an HTTP check on the console reports a healthy endpoint in
// both windows. Reading an attribute off the broker MBean is the broker
// answering for itself.
func (c *Conn) Ping(ctx context.Context) error {
	if c.jolokia == nil {
		return errConnectionDown
	}
	select {
	case <-c.closed:
		return errConnectionDown
	default:
	}

	attribute := "BrokerVersion"
	if c.tiers.product == artemis {
		attribute = "Version"
	}
	if _, err := c.jolokia.readString(ctx, c.names.brokerMBean(), attribute); err != nil {
		return err
	}
	return nil
}

// Close releases what the connection holds. The registry closes on disconnect
// and on shutdown, so the second call has to be the one that does nothing.
func (c *Conn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
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
		model.CapDestinationPurge,
		model.CapDestinationMove,

		model.CapSubscriptionList,
		model.CapSubscriptionCreate,
		model.CapSubscriptionDelete,
		model.CapSubscriptionLag,
	}
}

// open dials the console and settles which product and which tiers answered.
func open(ctx context.Context, profile model.ConnectionProfile) (*Conn, error) {
	config, err := configOf(profile)
	if err != nil {
		return nil, err
	}

	client, product, brokerName, err := probe(ctx, config)
	if err != nil {
		return nil, err
	}
	if config.brokerName != "" {
		brokerName = config.brokerName
	}

	conn := &Conn{
		jolokia: client,
		names:   names{product: product, broker: brokerName},
		config:  config,
		tiers:   tiers{product: product, amqpReason: probeAMQP(ctx, config)},
		closed:  make(chan struct{}),
	}
	conn.capabilities = conn.declare()
	return conn, nil
}

// declare turns the tiers into the capability set the pages gate on.
func (c *Conn) declare() model.Capabilities {
	declared := model.Capabilities{
		Supported: capabilities(),
		Degraded:  map[model.Capability]string{},
		Caveats:   map[model.Capability]string{},
	}
	return declared
}

// probe finds the agent and works out which broker is behind it.
//
// The product is read off the MBean domain rather than off a version string:
// each broker registers its whole tree under its own domain and neither
// answers for the other, so a search that returns names is proof rather than
// inference. The agent path is tried in the same step because the two travel
// together - Artemis mounts the agent under its Hawtio console, Classic beside
// its REST API - and a deployment behind a proxy can override the path while
// keeping the tree.
func probe(ctx context.Context, config clientConfig) (*jolokiaClient, product, string, error) {
	type candidate struct {
		path    string
		product product
		pattern string
	}
	candidates := []candidate{
		{artemisPath, artemis, artemisDomain + ":broker=*"},
		{classicPath, classic, classicDomain + ":type=Broker,brokerName=*"},
	}
	if config.agentPath != "" {
		// An explicit path still has to be told apart, so both trees are
		// tried against it rather than one being assumed.
		candidates = []candidate{
			{config.agentPath, artemis, artemisDomain + ":broker=*"},
			{config.agentPath, classic, classicDomain + ":type=Broker,brokerName=*"},
		}
	}

	var lastErr error
	for _, c := range candidates {
		client, err := newJolokiaClient(config.console, c.path, config.username, config.password,
			config.origin, config.timeout, config.skipVerify)
		if err != nil {
			return nil, "", "", err
		}

		found, err := client.search(ctx, c.pattern)
		if err != nil {
			lastErr = err
			if forbidden(err) {
				// Worth reporting as itself rather than being retried into a
				// "no broker here": the agent answered, and the credentials
				// or the Origin policy are what stood in the way.
				return nil, "", "", fmt.Errorf("the jolokia agent refused the call: %w", err)
			}
			continue
		}
		if name, ok := brokerNameFrom(found, c.product); ok {
			return client, c.product, name, nil
		}
	}

	if lastErr != nil {
		return nil, "", "", fmt.Errorf("no activemq broker answered at %s: %w", config.console, lastErr)
	}
	return nil, "", "", fmt.Errorf("no activemq broker answered at %s", config.console)
}

// brokerNameFrom picks the broker out of a search result.
//
// One JVM can register several brokers, and there is no rule saying which a
// profile meant - so several is a configuration the user has to resolve in the
// form rather than one this picks for them.
func brokerNameFrom(found []string, p product) (string, bool) {
	key := "brokerName"
	if p == artemis {
		key = "broker"
	}

	// Deduplicated, because a search returns one entry per MBean rather than
	// one per broker: Artemis alone answers broker=* with the broker, every
	// acceptor and every address under it, all carrying the same name.
	seen := make(map[string]struct{}, 2)
	for _, raw := range found {
		_, keys, err := parseObjectName(raw)
		if err != nil {
			continue
		}
		if name := keys[key]; name != "" {
			seen[name] = struct{}{}
		}
	}
	if len(seen) != 1 {
		return "", false
	}
	for name := range seen {
		return name, true
	}
	return "", false
}

// probeAMQP reports why the optional tier is unavailable, or "" when it is up.
func probeAMQP(ctx context.Context, config clientConfig) string {
	if strings.TrimSpace(config.amqpURL) == "" {
		return amqpAbsent
	}
	return dialAMQP(ctx, config)
}

// configOf reduces a profile to what this driver dials.
func configOf(profile model.ConnectionProfile) (clientConfig, error) {
	console := firstEndpoint(profile.Endpoints)
	if console == "" {
		return clientConfig{}, errors.New("no console address configured")
	}

	timeout := time.Duration(profile.TimeoutSec) * time.Second
	if profile.TimeoutSec <= 0 {
		timeout = 10 * time.Second
	}

	config := clientConfig{
		console:    console,
		agentPath:  normalisePath(profile.Option(OptionJolokiaPath)),
		brokerName: strings.TrimSpace(profile.Option(OptionBrokerName)),
		origin:     strings.TrimSpace(profile.Option(OptionOrigin)),
		amqpURL:    strings.TrimSpace(profile.Option(OptionAMQPURL)),
		timeout:    timeout,
		skipVerify: isTrue(profile.Option(OptionTLSSkipVerify)),
	}

	if profile.Auth.Mechanism != model.AuthNone {
		config.username = profile.Secret(SecretUsername)
		config.password = profile.Secret(SecretPassword)
	}

	// The console's credentials are the fallback, which is the common case:
	// most deployments authenticate both against the same realm.
	config.amqpUser = profile.Secret(SecretAMQPUsername)
	config.amqpPass = profile.Secret(SecretAMQPPassword)
	if config.amqpUser == "" {
		config.amqpUser, config.amqpPass = config.username, config.password
	}
	return config, nil
}

// firstEndpoint takes the console address out of the profile's list.
//
// The field is a list because every family's is, not because a second console
// would mean anything: two consoles are two brokers, and this driver reads one
// broker's tree.
func firstEndpoint(endpoints string) string {
	for _, part := range strings.Split(endpoints, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			return withScheme(trimmed)
		}
	}
	return ""
}

// withScheme accepts the host:port a user types out of habit, because every
// other family's endpoint field takes one and the muscle memory is real.
func withScheme(endpoint string) string {
	if strings.Contains(endpoint, "://") {
		return endpoint
	}
	return "http://" + endpoint
}

func normalisePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimSuffix(path, "/")
}

func isTrue(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}
