package connection

import (
	"fmt"
	"maps"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

// AddConnection adds and persists a connection profile.
//
// Everything family-specific arrives inside the profile - endpoints, options,
// secrets - so adding a broker family never widens this signature.
func (s *Service) AddConnection(input model.ConnectionProfile) (*model.ConnectionProfile, error) {
	// Before validation, because whether an address is required is the
	// family's answer and the default family has to be settled to ask it.
	kind := input.Kind
	if kind == "" {
		kind = model.KindRocketMQ
	}
	name, endpoints, err := s.validateConnectionFields(kind, input.Name, input.Endpoints, input.TimeoutSec)
	if err != nil {
		return nil, err
	}
	enableACL, accessKey, secretKey, err := normalizeACLConfig(
		input.ACLEnabled(), input.Secret(model.SecretAccessKey), input.Secret(model.SecretSecretKey))
	if err != nil {
		return nil, err
	}
	group, timeoutSec, remark := input.Group, input.TimeoutSec, input.Remark

	defer s.notifyChanged()
	s.mu.Lock()
	defer s.mu.Unlock()
	connection := &model.ConnectionProfile{
		ID:         s.nextID,
		Name:       name,
		Group:      normalizeConnectionGroup(group),
		Endpoints:  endpoints,
		TimeoutSec: timeoutSec,
		Kind:       kind,
		Options:    input.Options,
		Status:     model.StatusOffline,
		LastCheck:  "-",
		IsDefault:  len(s.connections) == 0,
		Remark:     remark,
	}
	applyCredentials(connection, input, enableACL, accessKey, secretKey)
	s.connections[connection.ID] = connection
	if err := s.saveConnectionsLocked(); err != nil {
		delete(s.connections, connection.ID)
		return nil, fmt.Errorf("failed to save connection config: %w", err)
	}
	s.nextID++
	copy := *connection
	return &copy, nil
}

/*
 * applyCredentials replaces the stored credentials with what the form
 * collected.
 *
 * Every driver's credentials, not only RocketMQ's access key pair. Those two
 * are normalised separately because they alone have an enable flag and a
 * global fallback in settings; everything else is just what the form asked
 * for. Keeping only the pair meant a RabbitMQ connection was saved with no
 * username and no password - and because the form's test button probes the
 * submitted profile rather than the stored one, it passed on the way in and
 * the connection could not open afterwards.
 *
 * Replaces rather than merges. The submission is the whole truth about
 * credentials, so a key it does not carry is gone. Writing only what arrived
 * meant clearing a credential did not clear it: SetACL removed the access key
 * pair by name, so RocketMQ looked right, while every other family's password
 * survived being cleared and the next connect used one the form had reported
 * as gone.
 *
 * Building a fresh map rather than editing the old one also keeps
 * UpdateConnection's rollback and its reconnect check honest: both compare
 * against a shallow copy taken before this runs, which shared the very map
 * this used to edit in place.
 */
func applyCredentials(
	connection *model.ConnectionProfile,
	input model.ConnectionProfile,
	enableACL bool,
	accessKey, secretKey string,
) {
	connection.Secrets = nil
	for key, value := range input.Secrets {
		if key == model.SecretAccessKey || key == model.SecretSecretKey {
			continue
		}
		connection.SetSecret(key, value)
	}
	connection.SetACL(enableACL, accessKey, secretKey)

	/*
	 * SetACL owns the mechanism, which is right for a family whose only one is
	 * ACL: off means anonymous. A family with a mechanism of its own - RabbitMQ
	 * authenticates with a plain user and password - has to keep the one it
	 * declared, or it is stored as an anonymous connection that cannot open.
	 */
	if !enableACL && input.Auth.Mechanism != "" {
		connection.Auth.Mechanism = input.Auth.Mechanism
	}
}

/*
 * dialParametersChanged reports whether an open client is now dialled with
 * something the stored profile no longer says.
 *
 * Options count. Every driver reads them in its configOf - a RocketMQ
 * namespace, a Kafka security protocol, a Pulsar tenant - so leaving them out
 * meant editing one and finding the connection still scoped to the old value
 * until the app was restarted. Nothing on screen said so.
 *
 * Name, group and remark deliberately do not: they are labels, and dropping a
 * working client to rename it would be a surprise, not a correction.
 */
func dialParametersChanged(previous, current model.ConnectionProfile) bool {
	return previous.Endpoints != current.Endpoints ||
		previous.TimeoutSec != current.TimeoutSec ||
		previous.ACLEnabled() != current.ACLEnabled() ||
		!maps.Equal(previous.Options, current.Options) ||
		!maps.Equal(previous.Secrets, current.Secrets)
}

// editedKind is the family the submission will be stored under.
//
// A form need not resubmit the kind, and a profile's family is not editable,
// so an empty one means the stored family rather than a new connection's
// default - which is what the address requirement has to be asked of.
func (s *Service) editedKind(id int, submitted model.MQKind) model.MQKind {
	if submitted != "" {
		return submitted
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if stored, exists := s.connections[id]; exists {
		return stored.Kind
	}
	return model.KindRocketMQ
}

// UpdateConnection updates and persists a connection profile.
func (s *Service) UpdateConnection(id int, input model.ConnectionProfile) (*model.ConnectionProfile, error) {
	name, endpoints, err := s.validateConnectionFields(
		s.editedKind(id, input.Kind), input.Name, input.Endpoints, input.TimeoutSec)
	if err != nil {
		return nil, err
	}
	enableACL, accessKey, secretKey := input.ACLEnabled(),
		input.Secret(model.SecretAccessKey), input.Secret(model.SecretSecretKey)
	group, timeoutSec, remark := input.Group, input.TimeoutSec, input.Remark

	defer s.notifyChanged()
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	s.mu.Lock()
	connection, exists := s.connections[id]
	if !exists {
		s.mu.Unlock()
		return nil, fmt.Errorf("connection not found: %d", id)
	}
	enableACL, accessKey, secretKey, err = normalizeACLConfig(enableACL, accessKey, secretKey)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	previous := *connection
	connection.Name = name
	connection.Group = normalizeConnectionGroup(group)
	connection.Endpoints = endpoints
	if input.Kind != "" {
		connection.Kind = input.Kind
	}
	if input.Options != nil {
		connection.Options = input.Options
	}
	connection.TimeoutSec = timeoutSec
	applyCredentials(connection, input, enableACL, accessKey, secretKey)
	connection.Remark = remark
	clientConfigChanged := dialParametersChanged(previous, *connection)
	wasOnline := previous.Status == model.StatusOnline
	if clientConfigChanged {
		connection.Status = model.StatusOffline
	}
	if err := s.saveConnectionsLocked(); err != nil {
		*connection = previous
		s.mu.Unlock()
		return nil, fmt.Errorf("failed to save connection config: %w", err)
	}
	result := *connection
	s.mu.Unlock()

	if clientConfigChanged && wasOnline {
		s.runtime.Remove(id)
		if err := s.connectRuntimeLocked(id); err != nil {
			return &result, fmt.Errorf("connection config saved, but reconnect with new config failed: %w", err)
		}
		return s.GetConnection(id)
	}
	return &result, nil
}

/*
 * SetOption replaces one stored option and redials a connection the change
 * invalidates.
 *
 * Distinct from UpdateConnection, which takes a whole form submission. A
 * caller changing one setting has no business resubmitting the credentials to
 * do it, and the round trip through the form is exactly where a stored secret
 * gets lost.
 *
 * An empty value removes the key rather than storing a blank, because Option
 * cannot tell the two apart and a blank one would sit in connections.json
 * saying nothing.
 */
func (s *Service) SetOption(id int, key, value string) (*model.ConnectionProfile, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("option key is required")
	}

	defer s.notifyChanged()
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	s.mu.Lock()
	connection, exists := s.connections[id]
	if !exists {
		s.mu.Unlock()
		return nil, fmt.Errorf("connection not found: %d", id)
	}
	previous := *connection
	options := maps.Clone(previous.Options)
	if options == nil {
		options = make(map[string]string, 1)
	}
	if value == "" {
		delete(options, key)
	} else {
		options[key] = value
	}
	// Setting an option to what it already holds must not drop a working
	// client: the switcher offers the current scope like any other.
	if maps.Equal(previous.Options, options) {
		result := *connection
		s.mu.Unlock()
		return &result, nil
	}
	connection.Options = options
	wasOnline := previous.Status == model.StatusOnline
	connection.Status = model.StatusOffline
	if err := s.saveConnectionsLocked(); err != nil {
		*connection = previous
		s.mu.Unlock()
		return nil, fmt.Errorf("failed to save connection config: %w", err)
	}
	result := *connection
	s.mu.Unlock()

	if wasOnline {
		s.runtime.Remove(id)
		if err := s.connectRuntimeLocked(id); err != nil {
			return &result, fmt.Errorf("connection config saved, but reconnect with new config failed: %w", err)
		}
		return s.GetConnection(id)
	}
	return &result, nil
}

// DeleteConnection removes a persisted connection profile.
func (s *Service) DeleteConnection(id int) error {
	defer s.notifyChanged()
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	connection, exists := s.connections[id]
	if !exists {
		return fmt.Errorf("connection not found: %d", id)
	}
	if connection.IsDefault && len(s.connections) > 1 {
		return fmt.Errorf("cannot delete the default connection; set another connection as default first")
	}

	deleted := *connection
	delete(s.connections, id)
	newDefaultID := 0
	if len(s.connections) > 0 && deleted.IsDefault {
		ids := sortedConnectionIDs(s.connections)
		s.connections[ids[0]].IsDefault = true
		newDefaultID = ids[0]
	}
	if err := s.saveConnectionsLocked(); err != nil {
		s.connections[id] = &deleted
		if newDefaultID != 0 {
			s.connections[newDefaultID].IsDefault = false
		}
		return fmt.Errorf("failed to save connection config: %w", err)
	}
	if deleted.Status == model.StatusOnline {
		s.runtime.Remove(id)
	}
	return nil
}

// SetDefaultConnection selects the default connection profile.
//
// Default is a stored flag and nothing more: it names the profile
// ConnectDefault opens on launch. It used to also move the runtime's one
// shared client, which is why it had a rollback path; several connections can
// be open at once now, so nothing runtime-side has to move.
func (s *Service) SetDefaultConnection(id int) error {
	defer s.notifyChanged()
	s.mu.Lock()
	defer s.mu.Unlock()

	connection, exists := s.connections[id]
	if !exists {
		return fmt.Errorf("connection not found: %d", id)
	}
	if connection.IsDefault {
		return nil
	}

	previousDefaultID := 0
	for _, current := range s.connections {
		if current.IsDefault {
			previousDefaultID = current.ID
			break
		}
	}
	for _, current := range s.connections {
		current.IsDefault = false
	}
	connection.IsDefault = true
	if err := s.saveConnectionsLocked(); err != nil {
		connection.IsDefault = false
		if previousDefaultID != 0 {
			s.connections[previousDefaultID].IsDefault = true
		}
		return fmt.Errorf("保存连接配置失败: %w", err)
	}
	return nil
}
