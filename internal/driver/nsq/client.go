package nsq

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// apiError is a call the daemon answered and refused.
//
// Worth a type rather than a formatted string because two of the codes are
// ordinary control flow: a topic or channel that is not on this nsqd is how a
// fan-out across a cluster reports "not here", and treating it as a failure
// would turn every partially-placed topic into an error.
type apiError struct {
	status  int
	code    string
	address string
}

func (e *apiError) Error() string {
	if e.code == "" {
		return fmt.Sprintf("%s answered HTTP %d", e.address, e.status)
	}
	return fmt.Sprintf("%s answered %s (HTTP %d)", e.address, e.code, e.status)
}

// The codes this driver reads rather than only reports. NSQ spells them in
// its own responses, which is why they are literals here.
const (
	codeTopicNotFound   = "TOPIC_NOT_FOUND"
	codeChannelNotFound = "CHANNEL_NOT_FOUND"
)

// notFound reports a call that failed only because the object is not on this
// daemon.
func notFound(err error) bool {
	var api *apiError
	if !errors.As(err, &api) {
		return false
	}
	return api.code == codeTopicNotFound || api.code == codeChannelNotFound
}

// httpClient talks to one daemon per call. NSQ has no session and no cookie,
// so the address is an argument rather than state.
type httpClient struct {
	http *http.Client
}

func newHTTPClient(timeout time.Duration, skipVerify bool) *httpClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &httpClient{http: &http.Client{Timeout: timeout, Transport: transport}}
}

// get reads JSON from one daemon. out may be nil for a call whose body is the
// plain "OK" that nsqd answers /ping with.
func (c *httpClient) get(ctx context.Context, address, path string, query url.Values, out any) error {
	return c.call(ctx, http.MethodGet, address, path, query, nil, out)
}

// post runs a management operation. Every one of them takes its arguments in
// the query string; body is the message payload and is nil everywhere else.
func (c *httpClient) post(ctx context.Context, address, path string, query url.Values, body []byte, out any) error {
	return c.call(ctx, http.MethodPost, address, path, query, body, out)
}

func (c *httpClient) call(
	ctx context.Context, method, address, path string, query url.Values, body []byte, out any,
) error {
	endpoint := strings.TrimSuffix(address, "/") + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	// Pins the response envelope. nsqd answers a request without it in the
	// pre-1.0 shape - a status_code / status_txt / data wrapper - and every
	// struct in this package is written against the modern one.
	request.Header.Set("Accept", "application/vnd.nsq; version=1.0")

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return &apiError{status: response.StatusCode, code: messageOf(payload), address: address}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("%s answered something that is not %s: %w", address, path, err)
	}
	return nil
}

// messageOf pulls NSQ's own name for a failure out of the body. An empty
// result means the daemon said nothing useful, which the status code then has
// to carry on its own.
func messageOf(payload []byte) string {
	var envelope struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return ""
	}
	return envelope.Message
}

// normaliseAddress accepts the host:port a user types out of habit, because
// every other family's endpoint field takes one.
func normaliseAddress(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	return strings.TrimSuffix(trimmed, "/")
}

// splitAddresses reads an address list the way every field in this app is
// written: commas, semicolons or whitespace, and blanks dropped.
func splitAddresses(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	addresses := make([]string, 0, len(parts))
	for _, part := range parts {
		if address := normaliseAddress(part); address != "" {
			addresses = append(addresses, address)
		}
	}
	return addresses
}

// hostPort is how a daemon is named on a board: the address without its
// scheme, which is what NSQ's own tools print and what the user typed.
func hostPort(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return address
	}
	return parsed.Host
}

// joinHostPort builds the address of a daemon nsqlookupd named. lookupd
// reports a broadcast address and a port separately, and the broadcast
// address is a bare host - so this is not a string concatenation an IPv6
// literal would survive.
func joinHostPort(host string, port int) string {
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}
