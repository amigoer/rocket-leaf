package googlepubsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	pubsub "cloud.google.com/go/pubsub/v2"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/amigoer/mq-studio/internal/model"
)

// clientConfig is the profile reduced to what this driver calls with.
type clientConfig struct {
	project string
	// emulator is a host:port rather than a URL: that is the shape the client
	// library's own PUBSUB_EMULATOR_HOST takes, and the shape gRPC dials.
	// Empty is the ordinary case and means the real service.
	emulator string
	prefix   string
	timeout  time.Duration

	// credentials is a service account key, decoded from the profile's
	// secret. Nil means Application Default Credentials, which is a deliberate
	// choice on a machine that already holds a Google identity rather than an
	// unfinished form.
	credentials []byte
}

// configOf reduces a profile to what this driver connects with.
//
// Endpoints is not read at all, and nothing is missing: this family has no
// address. The project takes its place, and it is required because every
// resource name is built from it - there is no default and no way to discover
// one from a credential that may reach several.
func configOf(profile model.ConnectionProfile) (clientConfig, error) {
	project := strings.TrimSpace(profile.Option(OptionProjectID))
	if project == "" {
		return clientConfig{}, errors.New("no Google Cloud project configured")
	}

	timeout := time.Duration(profile.TimeoutSec) * time.Second
	if profile.TimeoutSec <= 0 {
		timeout = 10 * time.Second
	}

	credentials := strings.TrimSpace(profile.Secret(SecretCredentialsJSON))
	if credentials != "" && !json.Valid([]byte(credentials)) {
		// Worth refusing here rather than at the first call: the client
		// library reports a malformed key as a credentials-source failure
		// naming no field, and the mistake is almost always a key pasted
		// with something else around it.
		return clientConfig{}, errors.New(
			"the service account key is not valid JSON; paste the whole key file, braces included")
	}

	config := clientConfig{
		project:  project,
		emulator: strings.TrimSpace(profile.Option(OptionEmulatorHost)),
		prefix:   strings.TrimSpace(profile.Option(OptionResourcePrefix)),
		timeout:  timeout,
	}
	if credentials != "" {
		config.credentials = []byte(credentials)
	}
	return config, nil
}

// newClient builds the API client the whole connection uses.
//
// Three ways to authenticate, and the difference is visible to the user rather
// than a fallback. A profile carrying a service account key uses exactly that
// key; one without uses Application Default Credentials, which is how this app
// runs on a machine that already holds a Google identity; and one naming an
// emulator uses none at all, because the emulator refuses to check one and
// gRPC would otherwise refuse to send without a transport credential.
func newClient(ctx context.Context, config clientConfig) (*pubsub.Client, error) {
	var options []option.ClientOption
	switch {
	case config.emulator != "":
		// The same set the library's own PUBSUB_EMULATOR_HOST hook installs.
		// Passing them per connection rather than reading the environment is
		// what lets one window hold an emulator and a real project at once.
		options = append(options,
			option.WithEndpoint(config.emulator),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			option.WithoutAuthentication(),
			option.WithTelemetryDisabled(),
		)
	case len(config.credentials) > 0:
		options = append(options, option.WithCredentialsJSON(config.credentials))
	}

	// Building the client can reach the network - Application Default
	// Credentials asks the metadata server on a GCE or GKE workload - so it
	// takes the profile's own timeout rather than running unbounded.
	dialCtx, cancel := context.WithTimeout(ctx, config.timeout)
	defer cancel()
	client, err := pubsub.NewClient(dialCtx, config.project, options...)
	if err != nil {
		return nil, fmt.Errorf("no Google credentials this connection could use: %w", err)
	}
	return client, nil
}

// notFound reports an error the service raised because the object is gone.
//
// Worth separating from any other failure: a listing that raced a delete is
// ordinary and the row is simply dropped, where an authentication or
// permission failure is something the user has to act on.
func notFound(err error) bool { return status.Code(err) == codes.NotFound }
