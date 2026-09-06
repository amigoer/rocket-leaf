package solace

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// restClient talks to the REST messaging interface, which is the only way this
// driver sends a message.
//
// A different port and a different protocol from SEMP, and not a second view
// of it: SEMP manages objects and carries no message data at all. This one
// takes an HTTP POST whose body is the message and whose path is where it
// goes - /QUEUE/<name> or /TOPIC/<topic> - and answers in XML rather than
// JSON when it refuses.
type restClient struct {
	http *http.Client
	base string
	up   credential
}

// restError is the messaging interface refusing a call.
type restError struct {
	Status int
	Reason string
	Detail string
}

func (e *restError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("solace rest %d: %s", e.Status, e.Reason)
	}
	return fmt.Sprintf("solace rest %d", e.Status)
}

func newRESTClient(base string, up credential, timeout time.Duration, skipVerify bool) *restClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // the user asked for it in the form
	}
	return &restClient{
		http: &http.Client{Timeout: timeout, Transport: transport},
		base: strings.TrimSuffix(base, "/"),
		up:   up,
	}
}

// probeQueueName is a name no broker has. It is only ever used to provoke a
// refusal, so it must not be creatable by accident: it reads as what it is if
// it ever reaches a log, and a broker that did somehow have it would take the
// probe's empty body onto it.
const probeQueueName = "mq-studio.probe.no.such.queue"

// probe reports whether this interface will take a send from this credential.
//
// A POST rather than a GET, and to a queue that does not exist. The interface
// answers 405 to a GET whatever the credential is, so a GET proves the port is
// a Solace listener and nothing about whether a send would work. A POST to a
// missing queue is refused before anything is stored, and the refusal is the
// answer: "Queue Not Found" means the credential was accepted.
func (c *restClient) probe(ctx context.Context) error {
	err := c.post(ctx, "/QUEUE/"+segment(probeQueueName), "text/plain", nil, nil)
	if err == nil || queueMissing(err) {
		return nil
	}
	return err
}

// queueMissing reports the broker having taken the request and found no such
// queue, which is this driver's success case when probing and a real failure
// when sending.
func queueMissing(err error) bool {
	var rerr *restError
	if !errors.As(err, &rerr) {
		return false
	}
	return rerr.Status == http.StatusBadRequest && strings.Contains(
		strings.ToLower(rerr.Reason), "not found")
}

// post sends one message. headers carry whatever the caller wants attached as
// user properties or as delivery settings.
func (c *restClient) post(
	ctx context.Context, path, contentType string, body []byte, headers map[string]string,
) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, reader)
	if err != nil {
		return err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if !c.up.empty() {
		request.SetBasicAuth(c.up.username, c.up.password)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= 400 {
		return decodeRESTError(response.StatusCode, payload)
	}
	return nil
}

// decodeRESTError turns the interface's XML refusal into something a caller
// can branch on.
//
// XML rather than JSON, and that is the interface rather than an old version:
// SEMP on the same broker answers JSON, and this one answers
// <solace-error-response> on every failure. A body that is not one is a proxy
// or a plain TCP listener answering instead.
func decodeRESTError(status int, payload []byte) error {
	var envelope struct {
		XMLName xml.Name `xml:"solace-error-response"`
		Reason  string   `xml:"reason"`
		Detail  string   `xml:"detail"`
	}
	if err := xml.Unmarshal(payload, &envelope); err != nil {
		return &restError{Status: status, Reason: strings.TrimSpace(http.StatusText(status))}
	}
	return &restError{
		Status: status,
		Reason: strings.TrimSpace(envelope.Reason),
		Detail: strings.TrimSpace(envelope.Detail),
	}
}
