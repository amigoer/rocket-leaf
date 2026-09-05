package model

import "slices"

// AuthMechanism is how a connection authenticates.
type AuthMechanism string

const (
	AuthNone      AuthMechanism = "none"
	AuthACL       AuthMechanism = "acl" // RocketMQ AccessKey / SecretKey
	AuthPlain     AuthMechanism = "plain"
	AuthSASLPlain AuthMechanism = "sasl-plain"
	AuthSASLScram AuthMechanism = "sasl-scram"
	AuthToken     AuthMechanism = "token"
	AuthMutualTLS AuthMechanism = "mtls"

	// AuthNKey is a challenge signed with an Ed25519 seed, and AuthCreds is
	// that seed packaged with a JWT in a file the client library reads.
	//
	// Neither is a variation on the pairs above. A token is a shared string the
	// server compares; an nkey proves possession of a private key against a
	// nonce the server issues, and a creds file additionally carries the claims
	// that say what the bearer may do. NATS is the family that has them.
	AuthNKey  AuthMechanism = "nkey"
	AuthCreds AuthMechanism = "creds"
)

// AuthConfig carries the non-secret half of a connection's credentials.
// The secret half lives in ConnectionProfile.Secrets.
type AuthConfig struct {
	Mechanism AuthMechanism `json:"mechanism"`
}

// ConnectionProfile is one saved connection, of any family.
//
// It replaces Connection, whose NameServer / EnableACL / AccessKey / SecretKey
// fields were RocketMQ concepts sitting in the shared schema.
type ConnectionProfile struct {
	ID         int              `json:"id"`
	Name       string           `json:"name"`
	Group      string           `json:"group"` // free-form label; empty means ungrouped
	Kind       MQKind           `json:"kind"`
	Endpoints  string           `json:"endpoints"` // driver parses; replaces NameServer
	TimeoutSec int              `json:"timeoutSec"`
	Auth       AuthConfig       `json:"auth"`
	Status     ConnectionStatus `json:"status"`
	LastCheck  string           `json:"lastCheck"`
	IsDefault  bool             `json:"isDefault"`
	Remark     string           `json:"remark"`

	// Options holds non-secret driver-specific settings, validated against the
	// driver's form schema. It is what lets a new family add fields without
	// changing the stored schema.
	Options map[string]string `json:"options"`

	// Secrets is encrypted at rest and never leaves the Go process. The bridge
	// replaces it with the list of configured key names.
	Secrets map[string]string `json:"-"`
}

// Option returns a driver-specific setting.
func (p *ConnectionProfile) Option(key string) string {
	return p.Options[key]
}

// Secret returns a stored credential.
func (p *ConnectionProfile) Secret(key string) string {
	return p.Secrets[key]
}

// ConfiguredSecrets lists the credential keys that hold a value. It is what
// the bridge sends instead of the secrets themselves, so a form can show
// "already set" without the value ever reaching the renderer.
//
// The result is sorted: map order would otherwise change between calls and
// make the renderer re-render on an unchanged connection.
func (p *ConnectionProfile) ConfiguredSecrets() []string {
	configured := make([]string, 0, len(p.Secrets))
	for key, value := range p.Secrets {
		if value != "" {
			configured = append(configured, key)
		}
	}
	slices.Sort(configured)
	return configured
}

// ACLEnabled reports whether the profile authenticates with a key pair.
func (p *ConnectionProfile) ACLEnabled() bool {
	return p.Auth.Mechanism == AuthACL
}

// SetSecret stores a credential, creating the map on first use. An empty
// value removes the key, so ConfiguredSecrets never reports a blank one.
func (p *ConnectionProfile) SetSecret(key, value string) {
	if value == "" {
		delete(p.Secrets, key)
		return
	}
	if p.Secrets == nil {
		p.Secrets = make(map[string]string, 2)
	}
	p.Secrets[key] = value
}

// SetOption stores a driver-specific setting, creating the map on first use.
func (p *ConnectionProfile) SetOption(key, value string) {
	if p.Options == nil {
		p.Options = make(map[string]string, 2)
	}
	p.Options[key] = value
}

// Clone returns a deep copy, so a caller cannot mutate stored state through
// the maps it hands back.
func (p *ConnectionProfile) Clone() *ConnectionProfile {
	if p == nil {
		return nil
	}
	copied := *p
	copied.Options = cloneStringMap(p.Options)
	copied.Secrets = cloneStringMap(p.Secrets)
	return &copied
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

// Secret keys the connection store uses.
//
// They double as the legacy on-disk field names, which is what makes the
// migration a rename rather than a re-encryption: the stored ENC: values move
// across untouched and the user never re-enters a credential.
//
// Reserved for RocketMQ's ACL, and not to be reused. They skip
// applyCredentials' generic loop and are written only through SetACL, so
// another family storing under these names has them silently cleared; and
// resolveACLCredentials fills them from global settings for any profile that
// named no mechanism. A cloud driver names its own - awsAccessKeyId and the
// like.
const (
	SecretAccessKey = "accessKey"
	SecretSecretKey = "secretKey"
)

// SetACL sets the mechanism and the key pair together.
//
// They are one decision, not three: a profile with the ACL mechanism and no
// keys, or keys with the mechanism off, are both states nothing should be able
// to produce. Turning ACL off clears the stored keys rather than orphaning
// them.
func (p *ConnectionProfile) SetACL(enabled bool, accessKey, secretKey string) {
	if !enabled {
		p.Auth.Mechanism = AuthNone
		p.SetSecret(SecretAccessKey, "")
		p.SetSecret(SecretSecretKey, "")
		return
	}
	p.Auth.Mechanism = AuthACL
	p.SetSecret(SecretAccessKey, accessKey)
	p.SetSecret(SecretSecretKey, secretKey)
}

// ConnectionRecord is the serialisable form of a profile.
//
// ConnectionProfile.Secrets is json:"-" so a credential can never reach the
// renderer by accident. Anything that deliberately persists or exports a
// profile goes through this record instead, which carries the secrets
// explicitly and still reads the pre-kind field names on the way in.
type ConnectionRecord struct {
	ConnectionProfile
	Secrets map[string]string `json:"secrets,omitempty"`

	// Read on load, never written. A file written before broker kinds existed
	// carries these instead of endpoints, auth and secrets.
	LegacyNameServer string `json:"nameServer,omitempty"`
	LegacyEnableACL  bool   `json:"enableACL,omitempty"`
	LegacyAccessKey  string `json:"accessKey,omitempty"`
	LegacySecretKey  string `json:"secretKey,omitempty"`
}

// NewConnectionRecord prepares a profile for storage.
func NewConnectionRecord(profile *ConnectionProfile) *ConnectionRecord {
	if profile == nil {
		return nil
	}
	record := &ConnectionRecord{ConnectionProfile: *profile}
	record.ConnectionProfile.Secrets = nil
	record.Secrets = cloneStringMap(profile.Secrets)
	return record
}

// Profile applies the pre-kind compatibility rules and returns the profile.
//
// Secrets are passed through as stored: the ENC: values move across untouched,
// which is what keeps the migration from asking anyone to re-enter a
// credential.
func (r *ConnectionRecord) Profile() *ConnectionProfile {
	if r == nil {
		return nil
	}
	profile := r.ConnectionProfile
	profile.Secrets = cloneStringMap(r.Secrets)

	if profile.Kind == "" {
		profile.Kind = KindRocketMQ
	}
	if profile.Endpoints == "" {
		profile.Endpoints = r.LegacyNameServer
	}
	if profile.Auth.Mechanism == "" {
		profile.Auth.Mechanism = AuthNone
		if r.LegacyEnableACL {
			profile.Auth.Mechanism = AuthACL
		}
	}
	if len(profile.Secrets) == 0 {
		profile.SetSecret(SecretAccessKey, r.LegacyAccessKey)
		profile.SetSecret(SecretSecretKey, r.LegacySecretKey)
	}
	return &profile
}

// Secret returns a stored credential.
//
// It shadows the embedded profile's accessor deliberately: a record keeps its
// credentials in its own map, and reading through the embedded profile would
// always find nothing.
func (r *ConnectionRecord) Secret(key string) string {
	return r.Secrets[key]
}

// SetSecret stores a credential on the record.
func (r *ConnectionRecord) SetSecret(key, value string) {
	if value == "" {
		delete(r.Secrets, key)
		return
	}
	if r.Secrets == nil {
		r.Secrets = make(map[string]string, 2)
	}
	r.Secrets[key] = value
}
