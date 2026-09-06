package solace

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amigoer/mq-studio/internal/model"
)

var errConnectionDown = errors.New("solace connection is not open")

// The reasons a capability can be missing here, as i18n keys rather than
// sentences. The renderer turns them into the user's language; an English
// frame around one would put the key itself on screen.
//
// They are split finer than "the rest interface is unavailable" for the reason
// IBM MQ splits its messaging reasons and MQTT splits its $SYS ones: "nothing
// answered on that port" sends a user to the broker's service configuration or
// to the connection form's REST address, "the broker refused this credential"
// sends them to the Message VPN's client-usernames, and one sentence covering
// both sends half of them to the wrong place.
const (
	restUnreachable = "mq.solace.degraded.restUnreachable"
	restForbidden   = "mq.solace.degraded.restForbidden"
)

// defaultTimeout is what a profile that named none gets. SEMP is quick, but a
// listing that walks several pages is several round trips.
const defaultTimeout = 10 * time.Second

// defaultVPN is the Message VPN every broker ships with, and the one an
// unnamed profile falls back to when the broker hosts several.
const defaultVPN = "default"

// tiers is what answered when the connection opened.
type tiers struct {
	// restReason is empty when the REST messaging interface is usable, and
	// otherwise the i18n key saying why it is not.
	restReason string
	// restURL is where a send goes. Empty when restReason is set.
	restURL string
}

// Conn is one live connection to one Message VPN on one broker.
//
// "One connection" is an HTTP client rather than a socket: every SEMP call is
// a request that stands alone, so there is nothing held open between them and
// nothing to reconnect. What is held is which Message VPN the profile settled
// on, because every path this driver builds names it.
type Conn struct {
	semp   *sempClient
	rest   *restClient
	config clientConfig
	vpn    string

	capabilities model.Capabilities
	closeOnce    sync.Once
	closed       chan struct{}
}

// clientConfig is the profile reduced to what this driver dials.
type clientConfig struct {
	semp       string
	vpn        string
	restURL    string
	admin      credential
	rest       credential
	timeout    time.Duration
	skipVerify bool
}

// Kind identifies the family.
func (c *Conn) Kind() model.MQKind { return model.KindSolace }

// Capabilities is what this endpoint can do.
func (c *Conn) Capabilities() model.Capabilities { return c.capabilities }

// MsgVPN is which Message VPN this connection is pointed at. Every path the
// driver builds names it, and the boards print it.
func (c *Conn) MsgVPN() string { return c.vpn }

// Ping asks the Message VPN for its own state.
//
// Not the SEMP root, and the distinction matters more here than it does
// elsewhere: SEMP answers for the broker whether or not any particular VPN is
// up, and - worse - a path naming a VPN that does not exist answers 200 with
// an empty collection rather than an error. A check on SEMP alone would report
// a healthy endpoint for a typo, and every board would then show an empty
// broker. This reads the VPN's own state, which only the VPN can fill.
func (c *Conn) Ping(ctx context.Context) error {
	if err := c.live(); err != nil {
		return err
	}
	state, err := c.vpnState(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(state, "up") {
		return fmt.Errorf("message vpn %s is %s", c.vpn, state)
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
	if c.semp == nil {
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

// open dials the broker, settles which Message VPN was meant, and probes the
// REST messaging tier.
func open(ctx context.Context, profile model.ConnectionProfile) (*Conn, error) {
	config, err := configOf(profile)
	if err != nil {
		return nil, err
	}

	client, err := newSEMPClient(config.semp, config.admin, config.timeout, config.skipVerify)
	if err != nil {
		return nil, err
	}

	conn := &Conn{semp: client, config: config, closed: make(chan struct{})}
	vpn, err := conn.resolveMsgVPN(ctx)
	if err != nil {
		return nil, err
	}
	conn.vpn = vpn

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("%s at %s did not answer: %w", vpn, config.semp, err)
	}
	conn.capabilities = conn.declare(conn.probeREST(ctx))
	return conn, nil
}

/*
 * declare turns what answered into the capability set the pages gate on.
 *
 * One thing varies. SEMP is one credential on one port and either answers or
 * does not, so everything it backs is simply there. The REST messaging
 * interface is a tier of its own - a different port, a different protocol and
 * a credential from a different directory - so what it backs is degraded with
 * a reason rather than dropped, which is the difference between a page that
 * explains itself and a page that is simply missing.
 */
func (c *Conn) declare(rest tiers) model.Capabilities {
	declared := model.Capabilities{
		Supported: capabilities(),
		Degraded:  map[model.Capability]string{},
		Caveats:   map[model.Capability]string{},
	}
	if rest.restReason != "" {
		for _, capability := range restCapabilities() {
			declared.Supported = without(declared.Supported, capability)
			declared.Degraded[capability] = rest.restReason
		}
	}
	return declared
}

// restCapabilities are the ones the second interface answers, and the ones
// that go degraded together when it will not take this credential.
func restCapabilities() []model.Capability {
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
 * probeREST reports where a send goes, or why it cannot go anywhere.
 *
 * The address comes from the broker unless the profile named one. The REST
 * listen port is a Message VPN setting rather than a broker-wide one - two
 * VPNs on one broker listen on two ports - so it is read from the VPN this
 * connection is for, and asking the user for it would be asking them to copy
 * out something the broker already knows.
 *
 * The probe then posts to a queue that does not exist and reads the refusal
 * rather than the answer, the way IBM MQ's does. That is deliberate: a name
 * that does exist would have to be guessed, and it would put a message on it.
 * A GET would be no probe at all - the interface answers 405 to one whatever
 * the credential is, so a broker that would refuse every send looks healthy.
 * What the refusals say apart: "Queue Not Found" is the interface open and the
 * credential accepted, which is the success case here; 403 is the credential
 * refused or the client-username shut down; anything else is nothing
 * answering on that port.
 */
func (c *Conn) probeREST(ctx context.Context) tiers {
	endpoint, err := c.restEndpoint(ctx)
	if err != nil {
		return tiers{restReason: restUnreachable}
	}

	client := newRESTClient(endpoint, c.config.rest, c.config.timeout, c.config.skipVerify)
	switch err := client.probe(ctx); {
	case err == nil:
		c.rest = client
		return tiers{restURL: endpoint}
	case restRefused(err):
		return tiers{restReason: restForbidden}
	default:
		return tiers{restReason: restUnreachable}
	}
}

/*
 * restEndpoint is the address a send goes to.
 *
 * A profile that named one is taken as given: a deployment behind an ingress
 * publishes the messaging interface somewhere the broker cannot know about.
 * Otherwise it is this Message VPN's own listen port on the SEMP host, which
 * is where a broker reached directly serves it.
 *
 * Which of the two ports is chosen follows the scheme SEMP itself was reached
 * on, rather than the enabled flags. Those flags are an intent rather than a
 * fact: a broker with no server certificate reports
 * serviceRestIncomingTlsEnabled true and has nothing listening on the TLS port
 * at all, so preferring TLS because it says "enabled" points every send at a
 * closed port on every developer installation there is. A deployment that put
 * TLS on SEMP has certificates and serves messages the same way; one reached
 * in the clear does not.
 */
func (c *Conn) restEndpoint(ctx context.Context) (string, error) {
	if named := strings.TrimSpace(c.config.restURL); named != "" {
		return withScheme(named), nil
	}

	var service struct {
		PlainTextEnabled bool `json:"serviceRestIncomingPlainTextEnabled"`
		PlainTextPort    int  `json:"serviceRestIncomingPlainTextListenPort"`
		TLSEnabled       bool `json:"serviceRestIncomingTlsEnabled"`
		TLSPort          int  `json:"serviceRestIncomingTlsListenPort"`
	}
	path := "/msgVpns/" + segment(c.vpn) +
		"?select=serviceRestIncomingPlainTextEnabled,serviceRestIncomingPlainTextListenPort," +
		"serviceRestIncomingTlsEnabled,serviceRestIncomingTlsListenPort"
	if err := c.semp.configGet(ctx, path, &service); err != nil {
		return "", err
	}

	host := hostOf(c.config.semp)
	secure := strings.HasPrefix(c.config.semp, "https://")
	plain := service.PlainTextEnabled && service.PlainTextPort > 0
	encrypted := service.TLSEnabled && service.TLSPort > 0

	switch {
	case secure && encrypted:
		return fmt.Sprintf("https://%s:%d", host, service.TLSPort), nil
	case !secure && plain:
		return fmt.Sprintf("http://%s:%d", host, service.PlainTextPort), nil
	case plain:
		return fmt.Sprintf("http://%s:%d", host, service.PlainTextPort), nil
	case encrypted:
		return fmt.Sprintf("https://%s:%d", host, service.TLSPort), nil
	default:
		return "", fmt.Errorf("%s does not serve the rest messaging interface", c.vpn)
	}
}

/*
 * resolveMsgVPN settles which Message VPN this profile meant.
 *
 * One broker hosts several. A name that was given is taken as given and
 * checked, so a typo fails here - where the message can name the field and
 * list what does exist - rather than at the first board that asks for a queue.
 * That check is not optional politeness: SEMP answers a collection under a
 * Message VPN that does not exist with 200 and an empty list, so an unchecked
 * name produces a connection that opens, reports nothing anywhere, and looks
 * like an empty broker.
 *
 * With no name, one VPN is unambiguous and several are not - except that every
 * broker ships "default" and it is what an unconfigured profile means, so it
 * wins when it is there. A broker with several and no "default" is a
 * configuration the user resolves in the form.
 */
func (c *Conn) resolveMsgVPN(ctx context.Context) (string, error) {
	wanted := strings.TrimSpace(c.config.vpn)
	names, err := c.listMsgVPNNames(ctx)
	if err != nil {
		return "", fmt.Errorf("no solace broker answered at %s: %w", c.config.semp, err)
	}

	if wanted != "" {
		for _, name := range names {
			if name == wanted {
				return name, nil
			}
		}
		return "", fmt.Errorf("%s has no message vpn named %q; it has %s",
			c.config.semp, wanted, strings.Join(names, ", "))
	}

	switch {
	case len(names) == 0:
		return "", fmt.Errorf("%s has no message vpn", c.config.semp)
	case len(names) == 1:
		return names[0], nil
	}
	for _, name := range names {
		if name == defaultVPN {
			return name, nil
		}
	}
	return "", fmt.Errorf("%s hosts %s; name the message vpn this connection is for",
		c.config.semp, strings.Join(names, ", "))
}

// listMsgVPNNames is every Message VPN the credential can see, sorted.
func (c *Conn) listMsgVPNNames(ctx context.Context) ([]string, error) {
	type vpnRow struct {
		MsgVpnName string `json:"msgVpnName"`
	}
	rows, err := listConfig[vpnRow](ctx, c.semp, "/msgVpns?select=msgVpnName")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if name := strings.TrimSpace(row.MsgVpnName); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// vpnState reads the one field that says this Message VPN is serving.
func (c *Conn) vpnState(ctx context.Context) (string, error) {
	var vpn struct {
		State   string `json:"state"`
		Enabled bool   `json:"enabled"`
	}
	path := "/msgVpns/" + segment(c.vpn) + "?select=state,enabled"
	if err := c.semp.monitorGet(ctx, path, &vpn); err != nil {
		if notFound(err) {
			return "", fmt.Errorf("%s no longer has a message vpn named %s", c.config.semp, c.vpn)
		}
		return "", err
	}
	if !vpn.Enabled {
		return "disabled", nil
	}
	if vpn.State == "" {
		return "unknown", nil
	}
	return vpn.State, nil
}

// configOf reduces a profile to what this driver dials.
func configOf(profile model.ConnectionProfile) (clientConfig, error) {
	semp := firstEndpoint(profile.Endpoints)
	if semp == "" {
		return clientConfig{}, errors.New("no semp address configured")
	}

	timeout := time.Duration(profile.TimeoutSec) * time.Second
	if profile.TimeoutSec <= 0 {
		timeout = defaultTimeout
	}

	config := clientConfig{
		semp:       semp,
		vpn:        strings.TrimSpace(profile.Option(OptionMsgVPN)),
		restURL:    strings.TrimSpace(profile.Option(OptionRESTURL)),
		timeout:    timeout,
		skipVerify: isTrue(profile.Option(OptionTLSSkipVerify)),
	}

	if profile.Auth.Mechanism != model.AuthNone {
		config.admin = credential{
			username: profile.Secret(SecretUsername),
			password: profile.Secret(SecretPassword),
		}
	}

	// No fallback to the management credential, deliberately. A SEMP user is
	// broker-wide and has an access level; a REST credential is a
	// client-username inside one Message VPN. They are different objects in
	// different directories, and a management username offered as one is
	// refused by every broker that checks. An empty pair sends no header at
	// all, which is what a VPN whose basic authentication type is none takes.
	config.rest = credential{
		username: profile.Secret(SecretRESTUsername),
		password: profile.Secret(SecretRESTPassword),
	}
	return config, nil
}

// firstEndpoint takes the SEMP address out of the profile's list.
//
// The field is a list because every family's is, not because a second address
// would mean anything: two brokers are two connections, and this driver reads
// one Message VPN on one of them.
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
// default is http rather than https: SEMP's plain-text listener is on by
// default and is what a developer installation serves, and a broker that has
// been given TLS is one whose address is written out in full.
func withScheme(endpoint string) string {
	if strings.Contains(endpoint, "://") {
		return endpoint
	}
	return "http://" + endpoint
}

// hostOf is the host and nothing else, which is what the REST messaging port
// is put on when the profile named no address for it.
func hostOf(endpoint string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	trimmed = strings.TrimSuffix(trimmed, "/")
	if slash := strings.IndexByte(trimmed, '/'); slash >= 0 {
		trimmed = trimmed[:slash]
	}
	if colon := strings.LastIndexByte(trimmed, ':'); colon >= 0 && !strings.Contains(trimmed[colon:], "]") {
		trimmed = trimmed[:colon]
	}
	return trimmed
}

func isTrue(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}

// restRefused reports the REST messaging interface rejecting the credential,
// as opposed to not answering at all.
func restRefused(err error) bool {
	var rerr *restError
	if !errors.As(err, &rerr) {
		return false
	}
	// 403 rather than 401, which is the broker's own choice: it answers a
	// wrong password and a shut-down client-username alike with Forbidden and
	// never challenges.
	return rerr.Status == http.StatusForbidden || rerr.Status == http.StatusUnauthorized
}
