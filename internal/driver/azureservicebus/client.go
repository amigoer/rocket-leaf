package azureservicebus

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus/admin"

	"github.com/amigoer/mq-studio/internal/model"
)

// clientConfig is the profile reduced to what this driver connects with.
type clientConfig struct {
	// namespace is the host the profile named, without a scheme and without a
	// trailing slash. It is what the connection row shows and what every error
	// message calls the endpoint.
	namespace string

	// connectionString is what both SDK clients are actually built from. It is
	// either the string the profile carried or one composed from the namespace
	// and the key pair, so nothing below this line has to know which.
	connectionString string

	// emulatorManagement is the host:port of an emulator's Atom management
	// port. Empty is the ordinary case and means the real service.
	emulatorManagement string

	prefix  string
	timeout time.Duration
}

// emulator reports whether this connection was pointed at an emulator.
func (c clientConfig) emulator() bool { return c.emulatorManagement != "" }

/*
 * configOf reduces a profile to what this driver connects with.
 *
 * Two ways to fill the form and one shape underneath. A pasted connection
 * string wins outright, because it carries an endpoint of its own and a
 * namespace typed beside it could disagree - and the string is what the user
 * copied out of the portal, so it is the one that is right. Otherwise the
 * string is composed from the namespace and the shared access key, which is
 * the same document the portal would have handed over.
 *
 * The namespace is required either way. It is what the connection list shows
 * in its address column, and a form that let it be omitted whenever a string
 * was pasted would leave a perfectly good connection unnamed in the list.
 */
func configOf(profile model.ConnectionProfile) (clientConfig, error) {
	namespace := namespaceOf(profile.Endpoints)
	if namespace == "" {
		return clientConfig{}, errors.New("no Service Bus namespace configured")
	}

	timeout := time.Duration(profile.TimeoutSec) * time.Second
	if profile.TimeoutSec <= 0 {
		timeout = 10 * time.Second
	}

	config := clientConfig{
		namespace:          namespace,
		emulatorManagement: strings.TrimSpace(profile.Option(OptionEmulatorManagement)),
		prefix:             strings.TrimSpace(profile.Option(OptionEntityPrefix)),
		timeout:            timeout,
	}

	if pasted := strings.TrimSpace(profile.Secret(SecretConnectionString)); pasted != "" {
		if !strings.Contains(pasted, "Endpoint=") {
			// Worth refusing here rather than at the first call: the SDK
			// reports a string with no Endpoint as "connection string does not
			// have an Endpoint key", which reads as a missing field on a form
			// that has no such field.
			return clientConfig{}, errors.New(
				"that does not look like a Service Bus connection string; " +
					"it has to start with Endpoint=sb://")
		}
		config.connectionString = withEmulatorFlag(pasted, config.emulator())
		return config, nil
	}

	key := strings.TrimSpace(profile.Secret(SecretSharedAccessKey))
	if key == "" {
		// There is no ambient credential to fall back on. SQS has the AWS
		// credential chain and Pub/Sub has Application Default Credentials, so
		// an empty key field there is a real way to run; here it is an
		// unfinished form, and saying so is better than a signature failure.
		return clientConfig{}, errors.New(
			"no credential: paste a connection string, or give the shared access key")
	}
	keyName := strings.TrimSpace(profile.Option(OptionSharedAccessKeyName))
	if keyName == "" {
		keyName = "RootManageSharedAccessKey"
	}

	config.connectionString = withEmulatorFlag(fmt.Sprintf(
		"Endpoint=sb://%s;SharedAccessKeyName=%s;SharedAccessKey=%s",
		namespace, keyName, key), config.emulator())
	return config, nil
}

// namespaceOf reads the host out of whatever the endpoint field holds.
//
// A user pastes three things into it: a bare host, a host:port for an
// emulator, and now and then the sb:// URL out of a connection string. All
// three name the same namespace, so all three are accepted and the port is
// kept - the emulator's AMQP port is part of its address.
func namespaceOf(endpoints string) string {
	first := strings.TrimSpace(endpoints)
	if index := strings.IndexAny(first, ",;\n"); index >= 0 {
		first = strings.TrimSpace(first[:index])
	}
	first = strings.TrimSuffix(first, "/")
	if first == "" {
		return ""
	}
	if parsed, err := url.Parse(first); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return first
}

// withEmulatorFlag adds the flag that lets the SDK dial a plaintext AMQP port.
//
// Without it the client insists on TLS on 5671 and refuses any host that is
// not a real namespace, which is the whole of what the emulator is not.
func withEmulatorFlag(connectionString string, emulator bool) string {
	if !emulator || strings.Contains(connectionString, "UseDevelopmentEmulator") {
		return connectionString
	}
	return strings.TrimSuffix(connectionString, ";") + ";UseDevelopmentEmulator=true"
}

// newClients builds the two clients one connection holds.
//
// Both are built from the same string, because a Service Bus credential is one
// credential: the shared access key that signs an AMQP claim signs an Atom
// request too. What differs is the transport, and only against an emulator.
func newClients(config clientConfig) (*azservicebus.Client, *admin.Client, error) {
	data, err := azservicebus.NewClientFromConnectionString(config.connectionString, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("this connection string does not name a namespace: %w", err)
	}

	var options *admin.ClientOptions
	if config.emulator() {
		options = &admin.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Transport: managementTransport{host: config.emulatorManagement},
			},
		}
	}
	management, err := admin.NewClientFromConnectionString(config.connectionString, options)
	if err != nil {
		_ = data.Close(context.Background())
		return nil, nil, fmt.Errorf("this connection string does not name a namespace: %w", err)
	}
	return data, management, nil
}

/*
 * managementTransport points the admin client at an emulator.
 *
 * The admin client composes every request as https://<namespace>/<path> and
 * offers no way to say otherwise - the host comes from the connection string's
 * Endpoint, and there is no endpoint-override option the way the AWS and
 * Google SDKs have. The emulator serves that same Atom API, unencrypted, on a
 * second port.
 *
 * So the one documented seam is used: azcore takes a Transporter, which is
 * handed the finished request. Scheme and host are rewritten and nothing else
 * is - the path, the query, the api-version and the signed Authorization
 * header are the SDK's own, and the signature covers the entity path rather
 * than the host, which is why the emulator accepts it.
 *
 * It is installed only for a connection that named an emulator management
 * host. A real namespace never reaches this type.
 */
type managementTransport struct{ host string }

func (t managementTransport) Do(request *http.Request) (*http.Response, error) {
	request.URL.Scheme = "http"
	request.URL.Host = t.host
	request.Host = t.host
	return http.DefaultClient.Do(request)
}

// Error classification.
//
// The SDK reports every failure as an *azservicebus.Error on the data plane
// and an *azcore.ResponseError on the management plane, so the two are asked
// separately and the callers only ever see the question they meant.

/*
 * notFound reports an error the service raised because the entity is gone.
 *
 * Worth separating from any other failure: a listing that raced a delete is
 * ordinary and the row is simply dropped, where an authentication or
 * permission failure is something the user has to act on.
 *
 * It answers for a delete, a create and every data-plane call. It does not
 * answer for a Get: the admin client's Get* methods return a nil response and
 * a nil error for an entity that does not exist, which is a convention rather
 * than a mistake and the reason every caller here checks the response for nil
 * before touching it. A driver that only checked the error would dereference
 * that nil on the first missing entity, which is a crashed window rather than
 * a "not found" on a page.
 */
func notFound(err error) bool {
	if err == nil {
		return false
	}
	var sbErr *azservicebus.Error
	if errors.As(err, &sbErr) && sbErr.Code == azservicebus.CodeNotFound {
		return true
	}
	return statusOf(err) == http.StatusNotFound
}

// alreadyExists reports the service refusing to create something twice.
func alreadyExists(err error) bool { return statusOf(err) == http.StatusConflict }

// statusOf is the HTTP status a management failure carried, or zero.
func statusOf(err error) int {
	var responseErr *azcore.ResponseError
	if errors.As(err, &responseErr) {
		return responseErr.StatusCode
	}
	return 0
}
