package ibmmq

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// restClient talks to the mqweb server, which hosts both of IBM MQ's HTTP
// interfaces on one port.
//
// Two interfaces rather than one, and they are not variations on each other:
// /ibmmq/rest/v1/admin describes objects as JSON and runs MQSC, and
// /ibmmq/rest/v1/messaging carries one message at a time as an HTTP body with
// its descriptor in headers. They also authorise separately - see conn.go -
// so this holds a credential per interface rather than one for the server.
type restClient struct {
	http    *http.Client
	base    string
	adminUP credential
	msgUP   credential
}

// credential is one HTTP Basic pair. Empty username means no Authorization
// header at all, which is what a server with no security configured takes.
type credential struct {
	username string
	password string
}

func (c credential) empty() bool { return strings.TrimSpace(c.username) == "" }

// csrfHeader is required on every request that is not a GET.
//
// Its value is never read: the mqweb server only checks that the header is
// present, because a browser cannot add a custom header to a cross-site form
// post. Omitting it fails the request with a 403 that says nothing about
// tokens, which is why it is set here rather than at each call site.
const csrfHeader = "ibm-mq-rest-csrf-token"

// restError is the mqweb server refusing a call.
//
// Every failure comes back as a JSON envelope with one or more entries, each
// carrying an IBM message id and, where a queue manager was involved, its own
// completion and reason codes. Both halves matter: MQWB0108E is a role that
// was never mapped and 2035 is the queue manager refusing an authorised
// caller, and they send a reader to entirely different places.
type restError struct {
	Status  int
	MsgID   string
	Message string
	Reason  int
}

func (e *restError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("ibm mq rest %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("ibm mq rest %d", e.Status)
}

// roleMissing reports the authenticated user not being mapped to the role the
// call needs. It is the one failure a user fixes in the mqweb configuration
// rather than in this app, and it is what the messaging tier is probed for.
func roleMissing(err error) bool {
	var rerr *restError
	if !errors.As(err, &rerr) {
		return false
	}
	return rerr.MsgID == "MQWB0108E"
}

// refused reports the credential itself being rejected, as opposed to being
// accepted and then found to hold the wrong role.
func refused(err error) bool {
	var rerr *restError
	if !errors.As(err, &rerr) {
		return false
	}
	return rerr.Status == http.StatusUnauthorized
}

// notFound reports an object the queue manager does not have. Callers turn it
// into an empty result or a named error rather than a failed page: a queue
// deleted between a listing and a read should read as gone.
func notFound(err error) bool {
	var rerr *restError
	if !errors.As(err, &rerr) {
		return false
	}
	return rerr.Status == http.StatusNotFound
}

// unsupportedMessage reports the mqweb server having found a message and
// refused to return its body.
//
// It is not an error about the queue or the request. The messaging interface
// carries character data only, so a message the queue manager stored in any
// other format - a dead letter, a PCF event, an application's own structure -
// is listed with its identifier and answered with 501 when opened.
func unsupportedMessage(err error) bool {
	var rerr *restError
	if !errors.As(err, &rerr) {
		return false
	}
	return rerr.MsgID == "MQWB0309E"
}

// newRESTClient builds the client for one mqweb server.
func newRESTClient(base string, admin, messaging credential, timeout time.Duration, skipVerify bool) (*restClient, error) {
	parsed, err := url.Parse(strings.TrimSuffix(base, "/"))
	if err != nil {
		return nil, fmt.Errorf("mqweb address %q is not a url: %w", base, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("mqweb address %q needs an http or https scheme", base)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // the user asked for it in the form
	}

	return &restClient{
		http:    &http.Client{Timeout: timeout, Transport: transport},
		base:    strings.TrimSuffix(parsed.String(), "/"),
		adminUP: admin,
		msgUP:   messaging,
	}, nil
}

// adminGet reads one administrative resource into out.
func (c *restClient) adminGet(ctx context.Context, path string, out any) error {
	body, _, err := c.do(ctx, http.MethodGet, c.base+"/ibmmq/rest/v1/admin"+path, c.adminUP, "", nil, nil)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("the admin api answered something unexpected for %s: %w", path, err)
	}
	return nil
}

// adminSend runs a POST, PATCH or DELETE against an administrative resource.
func (c *restClient) adminSend(ctx context.Context, method, path string, payload any) error {
	var body []byte
	contentType := ""
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body, contentType = encoded, "application/json"
	}
	_, _, err := c.do(ctx, method, c.base+"/ibmmq/rest/v1/admin"+path, c.adminUP, contentType, body, nil)
	return err
}

// messagingGet reads from the messaging interface, which answers with the
// message body and puts everything else in headers.
func (c *restClient) messagingGet(ctx context.Context, path string) ([]byte, http.Header, error) {
	return c.do(ctx, http.MethodGet, c.base+"/ibmmq/rest/v1/messaging"+path, c.msgUP, "", nil, nil)
}

// messagingPost sends one message. The headers carry the message descriptor
// and any user properties; the body is the message.
func (c *restClient) messagingPost(
	ctx context.Context, path, contentType string, body []byte, headers map[string]string,
) (http.Header, error) {
	_, responseHeaders, err := c.do(
		ctx, http.MethodPost, c.base+"/ibmmq/rest/v1/messaging"+path, c.msgUP, contentType, body, headers)
	return responseHeaders, err
}

// do is the one place a request is built, so the CSRF header and the
// credential cannot be forgotten at a call site.
func (c *restClient) do(
	ctx context.Context, method, endpoint string, up credential,
	contentType string, body []byte, headers map[string]string,
) ([]byte, http.Header, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, nil, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if method != http.MethodGet {
		// Any value will do; the server checks only that it is there.
		request.Header.Set(csrfHeader, "mq-studio")
	}
	if !up.empty() {
		request.SetBasicAuth(up.username, up.password)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, err
	}
	if response.StatusCode >= 400 {
		return nil, response.Header, decodeRESTError(response.StatusCode, payload)
	}
	return payload, response.Header, nil
}

// decodeRESTError turns the server's error envelope into something a caller
// can branch on.
//
// The envelope is a list because one MQSC command can fail several ways at
// once. The first entry is what is reported: it is the one the server put
// first, and the rest repeat its shape.
func decodeRESTError(status int, payload []byte) error {
	var envelope struct {
		Error []struct {
			MsgID      string `json:"msgId"`
			Message    string `json:"message"`
			ReasonCode int    `json:"reasonCode"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || len(envelope.Error) == 0 {
		// A failure with no envelope is Liberty answering rather than MQ -
		// most often an authentication challenge, whose body is HTML.
		return &restError{Status: status, Message: strings.TrimSpace(http.StatusText(status))}
	}
	first := envelope.Error[0]
	return &restError{
		Status:  status,
		MsgID:   first.MsgID,
		Message: first.Message,
		Reason:  first.ReasonCode,
	}
}
