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
