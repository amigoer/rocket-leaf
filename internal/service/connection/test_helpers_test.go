package connection

import (
	"github.com/amigoer/mq-studio/internal/model"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
)

var (
	initTestCryptoOnce sync.Once
	initTestCryptoErr  error
)

type fakeSettings struct {
	connectTimeout time.Duration
	autoConnect    bool
	accessKey      string
	secretKey      string
}

func (s fakeSettings) GetConnectTimeout() time.Duration {
	return s.connectTimeout
}

func (s fakeSettings) GetAutoConnectLast() bool {
	return s.autoConnect
}

func (s fakeSettings) GetGlobalACLCredentials() (string, string) {
	return s.accessKey, s.secretKey
}

func ensureTestCrypto(t *testing.T) {
	t.Helper()
	initTestCryptoOnce.Do(func() {
		initTestCryptoErr = crypto.InitKey(t.TempDir())
	})
	if initTestCryptoErr != nil {
		t.Fatalf("initialize test encryption key: %v", initTestCryptoErr)
	}
}

func newTestService(t *testing.T, settings Settings) *Service {
	t.Helper()
	ensureTestCrypto(t)
	if settings == nil {
		settings = fakeSettings{connectTimeout: 3 * time.Second, autoConnect: true}
	}
	return New(filepath.Join(t.TempDir(), "connections.json"), settings, noopRuntime{}, addressedEndpoints{})
}

// noopRuntime stands in where a test only exercises profile persistence. The
// tests that care about client lifecycle replace service.runtime with a fake
// that records calls.
type noopRuntime struct{}

func (noopRuntime) Connect(model.ConnectionProfile) error { return nil }
func (noopRuntime) HasClient(int) bool                    { return false }
func (noopRuntime) Remove(int)                            {}
func (noopRuntime) Test(model.ConnectionProfile) error    { return nil }
func (noopRuntime) CloseAll()                             {}

// addressedEndpoints is the policy every family had before one could
// decline an address, so a test using it reads exactly as it did.
type addressedEndpoints struct{}

func (addressedEndpoints) RequiresEndpoints(model.MQKind) bool { return true }

// hostedKind stands in for the six hosted families: reached by a region and a
// credential, with no broker address to dial.
const hostedKind = model.MQKind("hosted-fake")

// hostedDescriptor is the form such a family declares - no endpoint field at
// all. The policy below reads its answer off it rather than hard-coding one,
// which is what the composition root does with a registered driver.
var hostedDescriptor = model.DriverDescriptor{
	Kind: hostedKind,
	Form: []model.FormField{
		{Key: "region", Target: model.TargetOption, Required: true},
		{Key: "awsAccessKeyId", Target: model.TargetSecret, Required: true},
	},
}

type descriptorEndpoints struct{}

// RequiresEndpoints demands an address for a kind it does not know, as the
// driver-backed policy does for a kind with no driver.
func (descriptorEndpoints) RequiresEndpoints(kind model.MQKind) bool {
	if kind == hostedKind {
		return hostedDescriptor.RequiresEndpoints()
	}
	return true
}

func newHostedTestService(t *testing.T) *Service {
	t.Helper()
	ensureTestCrypto(t)
	return New(
		filepath.Join(t.TempDir(), "connections.json"),
		fakeSettings{connectTimeout: 3 * time.Second},
		noopRuntime{},
		descriptorEndpoints{},
	)
}

// hostedProfile is what a hosted family's form submits: a region and a
// credential, and no address.
func hostedProfile(name string) model.ConnectionProfile {
	profile := model.ConnectionProfile{
		Name:       name,
		Kind:       hostedKind,
		TimeoutSec: 5,
		Options:    map[string]string{"region": "eu-west-1"},
	}
	profile.SetSecret("awsAccessKeyId", "AKIA-example")
	return profile
}

// profileOf builds a profile from the arguments the old positional signature
// took, so these tests read as they did before AddConnection stopped spelling
// out one broker family's fields.
func profileOf(name, group, endpoints string, timeoutSec int, enableACL bool, accessKey, secretKey, remark string) model.ConnectionProfile {
	profile := model.ConnectionProfile{
		Name:       name,
		Group:      group,
		Kind:       model.KindRocketMQ,
		Endpoints:  endpoints,
		TimeoutSec: timeoutSec,
		Remark:     remark,
	}
	profile.SetACL(enableACL, accessKey, secretKey)
	return profile
}
