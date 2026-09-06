package solace

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

// sempClient talks to SEMP v2, which hosts both of the broker's management
// halves on one port.
//
// Two halves rather than two servers: /SEMP/v2/config creates, changes and
// deletes objects, /SEMP/v2/monitor reports their state and counts. They share
// a port, a credential and an envelope, so this holds one credential and
// differs only in the prefix.
type sempClient struct {
	http *http.Client
	base string
	up   credential
}

// credential is one HTTP Basic pair. Empty username means no Authorization
// header at all, which is what a broker with no management security takes.
type credential struct {
	username string
	password string
}

func (c credential) empty() bool { return strings.TrimSpace(c.username) == "" }

// sempError is SEMP refusing a call.
//
// Status is the broker's own word for the failure - NOT_FOUND, UNAUTHORIZED,
// NOT_ALLOWED - and it is what callers branch on, because the HTTP status is
// not reliable here. SEMP answers a missing object with HTTP 400 and puts
// NOT_FOUND in the envelope, so a driver keying off the HTTP code alone would
// report every deleted queue as a bad request.
type sempError struct {
	HTTPStatus  int
	Status      string
	Code        int
	Description string
}

func (e *sempError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("semp %s: %s", strings.ToLower(e.Status), e.Description)
	}
	return fmt.Sprintf("semp failed with %d", e.HTTPStatus)
}

// SEMP's own word for the one failure this driver has to tell from the rest.
const statusNotFound = "NOT_FOUND"

// notFound reports an object the broker does not have. Callers turn it into an
// empty result or a named error rather than a failed page: a queue deleted
// between a listing and a read should read as gone.
func notFound(err error) bool { return statusIs(err, statusNotFound) }

func statusIs(err error, status string) bool {
	var serr *sempError
	if !errors.As(err, &serr) {
		return false
	}
	return serr.Status == status
}

// newSEMPClient builds the client for one broker.
func newSEMPClient(base string, up credential, timeout time.Duration, skipVerify bool) (*sempClient, error) {
	parsed, err := url.Parse(strings.TrimSuffix(base, "/"))
	if err != nil {
		return nil, fmt.Errorf("semp address %q is not a url: %w", base, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("semp address %q needs an http or https scheme", base)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("semp address %q names no host", base)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // the user asked for it in the form
	}

	return &sempClient{
		http: &http.Client{Timeout: timeout, Transport: transport},
		base: strings.TrimSuffix(parsed.String(), "/"),
		up:   up,
	}, nil
}

// The two halves of SEMP, as URL prefixes.
const (
	configAPI  = "/SEMP/v2/config"
	monitorAPI = "/SEMP/v2/monitor"
)

// configGet reads one configuration object into out.
func (c *sempClient) configGet(ctx context.Context, path string, out any) error {
	return c.get(ctx, configAPI+path, out)
}

// monitorGet reads one monitored object into out.
func (c *sempClient) monitorGet(ctx context.Context, path string, out any) error {
	return c.get(ctx, monitorAPI+path, out)
}

func (c *sempClient) get(ctx context.Context, path string, out any) error {
	body, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("semp answered something unexpected for %s: %w", path, err)
	}
	return nil
}

// configSend runs a POST, PATCH, PUT or DELETE against a configuration object.
func (c *sempClient) configSend(ctx context.Context, method, path string, payload any) error {
	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = encoded
	}
	_, _, err := c.do(ctx, method, configAPI+path, body)
	return err
}

// maxPages bounds a paged read.
//
// SEMP hands back a cursor on the last page as readily as on any other, so
// "keep going while there is a cursor" is a loop with no end on a broker that
// keeps answering. The cap is high enough that no realistic broker reaches it
// with the page size below, and it is the difference between a slow page and a
// hung one.
const maxPages = 200

// pageSize is how many objects one read asks for. SEMP's own default is 10,
// which would turn a hundred queues into ten round trips.
const pageSize = 100

// listConfig reads a whole configuration collection, following the cursor.
func listConfig[T any](ctx context.Context, c *sempClient, path string) ([]T, error) {
	return listPaged[T](ctx, c, configAPI+path)
}

/*
 * listPaged walks one collection to its end.
 *
 * The cursor is an opaque string SEMP hands back in meta.paging, and it is
 * passed on verbatim rather than re-derived: it encodes the broker's own
 * index into the collection, so anything composed here would resume somewhere
 * else. It also has to be escaped as a query value, because it contains a
 * whole SEMP v1 request in XML.
 *
 * The loop stops on an empty page as well as on a missing cursor. A broker
 * that keeps offering one past the end would otherwise be read forever.
 */
func listPaged[T any](ctx context.Context, c *sempClient, path string) ([]T, error) {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}

	var all []T
	cursor := ""
	for page := 0; page < maxPages; page++ {
		request := fmt.Sprintf("%s%scount=%d", path, separator, pageSize)
		if cursor != "" {
			request += "&cursor=" + url.QueryEscape(cursor)
		}

		body, meta, err := c.do(ctx, http.MethodGet, request, nil)
		if err != nil {
			return nil, err
		}
		var batch []T
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("semp answered something unexpected for %s: %w", path, err)
		}
		all = append(all, batch...)

		if len(batch) == 0 || meta.Paging == nil || meta.Paging.CursorQuery == "" {
			return all, nil
		}
		cursor = meta.Paging.CursorQuery
	}
	return all, nil
}

// envelope is what every SEMP call answers with, success or failure.
type envelope struct {
	Data json.RawMessage `json:"data"`
	Meta sempMeta        `json:"meta"`
}

type sempMeta struct {
	ResponseCode int `json:"responseCode"`
	Error        *struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
		Status      string `json:"status"`
	} `json:"error"`
	Paging *struct {
		CursorQuery string `json:"cursorQuery"`
	} `json:"paging"`
}

// do is the one place a request is built, so the credential cannot be
// forgotten at a call site.
//
// It returns the meta block alongside the data because the paged reader needs
// the cursor out of it, and because the failure lives there rather than in the
// HTTP status.
func (c *sempClient) do(ctx context.Context, method, path string, body []byte) (json.RawMessage, sempMeta, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, sempMeta{}, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if !c.up.empty() {
		request.SetBasicAuth(c.up.username, c.up.password)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return nil, sempMeta{}, err
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, sempMeta{}, err
	}

	var decoded envelope
	if err := json.Unmarshal(payload, &decoded); err != nil {
		// No envelope at all is something other than SEMP answering - a proxy,
		// or an https endpoint reached over http.
		return nil, sempMeta{}, fmt.Errorf(
			"%s did not answer with semp (http %d)", c.base+path, response.StatusCode)
	}
	if decoded.Meta.Error != nil {
		return nil, decoded.Meta, &sempError{
			HTTPStatus:  response.StatusCode,
			Status:      decoded.Meta.Error.Status,
			Code:        decoded.Meta.Error.Code,
			Description: decoded.Meta.Error.Description,
		}
	}
	if response.StatusCode >= 400 {
		return nil, decoded.Meta, &sempError{HTTPStatus: response.StatusCode}
	}
	return decoded.Data, decoded.Meta, nil
}

// segment escapes one object name for a URL path.
//
// Necessary rather than tidy: a Solace object name may contain a slash, a hash
// or a space, and all three are ordinary in topic-shaped queue names. Without
// this a queue called "orders/eu" is read as a collection called "orders" with
// a sub-resource, which answers 404 for a queue that is plainly in the listing.
func segment(name string) string { return url.PathEscape(name) }
