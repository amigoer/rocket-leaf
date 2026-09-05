package activemq

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// jolokiaClient talks to the agent both brokers ship under their web console.
//
// Jolokia is a JMX-over-HTTP bridge, not a REST API with a model: every call
// is one of four commands - read an attribute, exec an operation, search for
// MBean names, list the tree - carrying an ObjectName. That is why this is the
// standard library and a hundred lines rather than a client library; there is
// no vocabulary here for one to wrap.
type jolokiaClient struct {
	http     *http.Client
	endpoint string
	username string
	password string
	origin   string
}

// defaultOrigin is what this driver claims to be when it calls.
//
// Both brokers ship jolokia-access.xml with <strict-checking/>, which refuses
// a request that carries no Origin header as coming from the null origin. The
// refusal is an HTTP 403 whose body says "Origin null is not allowed to call
// this agent" - indistinguishable at a glance from bad credentials, and the
// reason every request here sets the header whether or not the deployment
// checks it.
const defaultOrigin = "http://localhost"

// jolokiaError is the agent answering with a failure.
//
// Jolokia answers HTTP 200 and puts the real outcome in the body, so a caller
// that checks only the transport sees every broker-side failure as success.
// The status is a JMX-shaped code: 404 for an MBean that is not registered,
// 403 for a policy refusal, 500 for an operation that threw.
type jolokiaError struct {
	Status    int
	ErrorType string
	Message   string
	Request   string
}

func (e *jolokiaError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("jolokia %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("jolokia %d on %s", e.Status, e.Request)
}

// notRegistered reports the MBean having been absent, which is how both
// brokers answer for a destination that does not exist. Callers turn it into
// a not-found rather than an error, so a deleted queue reads as gone instead
// of as a broken page.
func notRegistered(err error) bool {
	var jerr *jolokiaError
	if !errors.As(err, &jerr) {
		return false
	}
	return jerr.Status == http.StatusNotFound ||
		strings.Contains(jerr.ErrorType, "InstanceNotFoundException")
}

// forbidden reports the agent's policy having refused the call. Separated from
// every other failure because it is the one a user can fix, in
// jolokia-access.xml, and the message alone does not say so.
func forbidden(err error) bool {
	var jerr *jolokiaError
	if !errors.As(err, &jerr) {
		return false
	}
	return jerr.Status == http.StatusForbidden || jerr.Status == http.StatusUnauthorized
}

// request is one Jolokia command.
type request struct {
	Type      string `json:"type"`
	MBean     string `json:"mbean,omitempty"`
	Attribute string `json:"attribute,omitempty"`
	Operation string `json:"operation,omitempty"`
	Arguments []any  `json:"arguments,omitempty"`

	// Path narrows a read to one field of a composite attribute, which saves
	// pulling a whole MBean over the wire to look at one number.
	Path string `json:"path,omitempty"`
}

// response is one result. Value stays raw so a caller decodes it into the
// shape it expects rather than through map[string]any.
type response struct {
	Status    int             `json:"status"`
	Value     json.RawMessage `json:"value"`
	ErrorType string          `json:"error_type"`
	Error     string          `json:"error"`
}

func readAttribute(mbean, attribute string) request {
	return request{Type: "read", MBean: mbean, Attribute: attribute}
}

func execOperation(mbean, operation string, arguments ...any) request {
	// Nil rather than an empty slice: Jolokia rejects "arguments": [] on an
	// operation that takes none, and omitempty only drops a nil.
	if len(arguments) == 0 {
		arguments = nil
	}
	return request{Type: "exec", MBean: mbean, Operation: operation, Arguments: arguments}
}

func searchMBeans(pattern string) request {
	return request{Type: "search", MBean: pattern}
}

// call sends one request and returns its value.
func (c *jolokiaClient) call(ctx context.Context, req request) (json.RawMessage, error) {
	results, err := c.batch(ctx, []request{req})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// batch sends several requests in one round trip.
//
// Jolokia takes an array and answers with an array in the same order, which is
// what keeps a page that reads twenty queues' depths to one HTTP call instead
// of twenty. A member failing does not fail the batch - each result carries
// its own status - so this returns the first failure and lets a caller that
// tolerates partial results use batchTolerant instead.
func (c *jolokiaClient) batch(ctx context.Context, requests []request) ([]json.RawMessage, error) {
	results, errs, err := c.batchTolerant(ctx, requests)
	if err != nil {
		return nil, err
	}
	for _, e := range errs {
		if e != nil {
			return nil, e
		}
	}
	return results, nil
}

// batchTolerant is batch for callers that expect some members to fail - most
// often a read across destinations where one was deleted between the search
// and the read. The two slices are the same length as requests.
func (c *jolokiaClient) batchTolerant(ctx context.Context, requests []request) ([]json.RawMessage, []error, error) {
	if len(requests) == 0 {
		return nil, nil, nil
	}

	body, err := json.Marshal(requests)
	if err != nil {
		return nil, nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Omitted only when a caller built the struct without one, which is how
	// the live test proves the header is what the broker's policy turns on.
	// Every client this package constructs carries defaultOrigin.
	if c.origin != "" {
		httpReq.Header.Set("Origin", c.origin)
	}
	if c.username != "" {
		httpReq.SetBasicAuth(c.username, c.password)
	}

	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		return nil, nil, &jolokiaError{
			Status:  httpResp.StatusCode,
			Message: http.StatusText(httpResp.StatusCode),
			Request: requests[0].MBean,
		}
	}

	// A single-element array comes back as a bare object from some agent
	// versions, so decode into a value that accepts either.
	raw, err := decodeResponses(httpResp.Body)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) != len(requests) {
		return nil, nil, fmt.Errorf("jolokia answered %d results for %d requests", len(raw), len(requests))
	}

	values := make([]json.RawMessage, len(raw))
	errs := make([]error, len(raw))
	for i, result := range raw {
		if result.Status != http.StatusOK {
			errs[i] = &jolokiaError{
				Status:    result.Status,
				ErrorType: result.ErrorType,
				Message:   result.Error,
				Request:   requests[i].MBean,
			}
			continue
		}
		values[i] = result.Value
	}
	return values, errs, nil
}

func decodeResponses(body interface{ Read([]byte) (int, error) }) ([]response, error) {
	decoder := json.NewDecoder(body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("jolokia answered something that is not json: %w", err)
	}

	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var single response
		if err := json.Unmarshal(raw, &single); err != nil {
			return nil, err
		}
		return []response{single}, nil
	}

	var many []response
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, err
	}
	return many, nil
}

// readString, readInt64 and readBool are the three shapes attributes come in.
// They exist because a JMX attribute read returns a bare JSON scalar, and
// unmarshalling one at every call site is where a wrong type would hide.
func (c *jolokiaClient) readString(ctx context.Context, mbean, attribute string) (string, error) {
	value, err := c.call(ctx, readAttribute(mbean, attribute))
	if err != nil {
		return "", err
	}
	var out string
	if err := json.Unmarshal(value, &out); err != nil {
		return "", fmt.Errorf("%s.%s is not a string: %w", mbean, attribute, err)
	}
	return out, nil
}

// search returns the ObjectNames matching a pattern, or an empty slice.
//
// An empty result is not an error: it is how the broker says it has no
// destinations, no acceptors, no subscriptions. Both brokers answer a search
// that matched nothing with a null value rather than an empty array.
func (c *jolokiaClient) search(ctx context.Context, pattern string) ([]string, error) {
	value, err := c.call(ctx, searchMBeans(pattern))
	if err != nil {
		if notRegistered(err) {
			return nil, nil
		}
		return nil, err
	}
	var found []string
	if err := json.Unmarshal(value, &found); err != nil {
		return nil, fmt.Errorf("jolokia search for %q did not return names: %w", pattern, err)
	}
	return found, nil
}

// newJolokiaClient builds the client for one endpoint.
//
// The endpoint is the console's base URL with the agent path already on it,
// because which path that is depends on which product answered and probe has
// settled that by the time this is called.
func newJolokiaClient(base, agentPath, username, password, origin string, timeout time.Duration, skipVerify bool) (*jolokiaClient, error) {
	parsed, err := url.Parse(strings.TrimSuffix(base, "/"))
	if err != nil {
		return nil, fmt.Errorf("console address %q is not a url: %w", base, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("console address %q needs an http or https scheme", base)
	}
	if origin == "" {
		origin = defaultOrigin
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // the user asked for it in the form
	}

	return &jolokiaClient{
		http:     &http.Client{Timeout: timeout, Transport: transport},
		endpoint: strings.TrimSuffix(parsed.String(), "/") + agentPath,
		username: username,
		password: password,
		origin:   origin,
	}, nil
}
