package kinesis

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awskinesis "github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"

	"github.com/amigoer/mq-studio/internal/model"
)

// clientConfig is the profile reduced to what this driver signs with.
type clientConfig struct {
	region string
	// endpoint overrides the regional one. Empty is the ordinary case.
	endpoint string
	prefix   string
	timeout  time.Duration

	// static is true when the profile carries its own key pair. False means
	// the SDK's default credential chain, which is a deliberate choice on a
	// machine that already has a role rather than an unfinished form.
	static       bool
	accessKeyID  string
	secretKey    string
	sessionToken string
}

// configOf reduces a profile to what this driver signs for.
//
// Endpoints is not read at all, and nothing is missing: this family has no
// address. The region takes its place, and it is required because SigV4 signs
// with it - a request signed for one region is refused by another.
func configOf(profile model.ConnectionProfile) (clientConfig, error) {
	region := strings.TrimSpace(profile.Option(OptionRegion))
	if region == "" {
		return clientConfig{}, errors.New("no AWS region configured")
	}

	timeout := time.Duration(profile.TimeoutSec) * time.Second
	if profile.TimeoutSec <= 0 {
		timeout = 10 * time.Second
	}

	accessKeyID := strings.TrimSpace(profile.Secret(SecretAccessKeyID))
	secretKey := strings.TrimSpace(profile.Secret(SecretSecretAccessKey))
	// Half a pair signs nothing, and falling through to the ambient chain
	// would quietly connect as whoever the machine is rather than as the
	// account the form names.
	if (accessKeyID == "") != (secretKey == "") {
		return clientConfig{}, errors.New(
			"an AWS credential needs both an access key id and a secret access key")
	}

	return clientConfig{
		region:       region,
		endpoint:     strings.TrimSpace(profile.Option(OptionEndpointURL)),
		prefix:       strings.TrimSpace(profile.Option(OptionStreamPrefix)),
		timeout:      timeout,
		static:       accessKeyID != "",
		accessKeyID:  accessKeyID,
		secretKey:    secretKey,
		sessionToken: strings.TrimSpace(profile.Secret(SecretSessionToken)),
	}, nil
}

// httpTimeoutHeadroom is added to the profile's timeout for the transport.
//
// Every call this driver makes carries the caller's deadline already, so the
// transport's own timeout exists only to stop a hung socket outliving the
// connection. The headroom keeps it from firing first and replacing the
// caller's error with a transport one that says less.
const httpTimeoutHeadroom = 5 * time.Second

// newClient builds the API client the whole connection uses.
//
// Two credential sources, and the difference is visible to the user rather
// than a fallback. A profile with a key pair signs with exactly that pair; one
// without uses the SDK's default chain - environment variables, the shared
// config file, an instance or container role - which is how this app runs on a
// machine that already has an AWS identity.
//
// Loading that chain can reach the network, so it takes the profile's own
// timeout rather than running unbounded.
func newClient(ctx context.Context, config clientConfig) (*awskinesis.Client, error) {
	options := awskinesis.Options{
		Region:     config.region,
		HTTPClient: &http.Client{Timeout: config.timeout + httpTimeoutHeadroom},
	}
	if config.endpoint != "" {
		options.BaseEndpoint = aws.String(config.endpoint)
	}

	if config.static {
		options.Credentials = credentials.NewStaticCredentialsProvider(
			config.accessKeyID, config.secretKey, config.sessionToken)
		return awskinesis.New(options), nil
	}

	loadCtx, cancel := context.WithTimeout(ctx, config.timeout)
	defer cancel()
	loaded, err := awsconfig.LoadDefaultConfig(loadCtx, awsconfig.WithRegion(config.region))
	if err != nil {
		return nil, fmt.Errorf("no AWS credentials on this machine: %w", err)
	}
	options.Credentials = loaded.Credentials
	return awskinesis.New(options), nil
}

// notFound reports an error the service raised because the stream, the shard
// or the consumer is gone.
//
// Worth separating from any other failure: a listing that raced a delete is
// ordinary and the row is simply dropped, where a signing or permission
// failure is something the user has to act on.
func notFound(err error) bool {
	var missing *types.ResourceNotFoundException
	return errors.As(err, &missing)
}
