package app

import (
	"context"
	"fmt"
	"time"

	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/connection"
)

// registryRuntime binds connection lifecycle to the driver registry.
//
// It lives in the composition root because it is the only place allowed to
// know both halves: the connection service defines what a client registry has
// to do, the drivers know how to do it, and neither imports the other.
//
// Nothing here is RocketMQ-specific any more. A profile names its own family
// and the registry looks the driver up, which is what lets a second family be
// opened by adding a driver rather than a branch.
type registryRuntime struct {
	registry *driver.Registry
}

func newRegistryRuntime(registry *driver.Registry) connection.ClientRuntime {
	return &registryRuntime{registry: registry}
}

// descriptorEndpoints answers the connection service's endpoint question from
// the driver's own form, and lives here for the same reason registryRuntime
// does: only the composition root may know both halves.
type descriptorEndpoints struct{}

func newDescriptorEndpoints() connection.EndpointPolicy { return descriptorEndpoints{} }

// RequiresEndpoints demands an address for a kind no driver is compiled in
// for. Such a profile cannot be opened anyway, and letting it save without one
// would only replace the error the user can act on with a later one they
// cannot.
func (descriptorEndpoints) RequiresEndpoints(kind model.MQKind) bool {
	d, ok := driver.Lookup(kind)
	if !ok {
		return true
	}
	return d.Descriptor().RequiresEndpoints()
}

// dialTimeout is how long opening or testing one profile may take. The profile
// arrives resolved, so a zero here means the profile carried no timeout.
func dialTimeout(profile model.ConnectionProfile) time.Duration {
	if profile.TimeoutSec > 0 {
		return time.Duration(profile.TimeoutSec) * time.Second
	}
	return defaultDialTimeout
}

const defaultDialTimeout = 5 * time.Second

func (r *registryRuntime) Connect(profile model.ConnectionProfile) error {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout(profile))
	defer cancel()
	if err := r.registry.Open(ctx, profile); err != nil {
		return err
	}
	// The most recently opened connection is what background sampling reads
	// when no page has named one.
	return r.registry.SetActive(profile.ID)
}

func (r *registryRuntime) HasClient(id int) bool {
	_, ok := r.registry.Get(id)
	return ok
}

func (r *registryRuntime) Remove(id int) {
	r.registry.Close(id)
	// Closing the connection the collector was sampling leaves it with nothing
	// to read, so hand it another open one rather than stopping until the user
	// happens to click a tab.
	if r.registry.ActiveID() != 0 {
		return
	}
	for _, remaining := range r.registry.IDs() {
		if err := r.registry.SetActive(remaining); err == nil {
			return
		}
	}
}

// Test opens the profile through its own driver, pings it and closes it.
//
// Going through the driver rather than a family-specific probe is what makes
// "test" mean the same thing as "connect": if this succeeds, Connect dials the
// identical parameters.
func (r *registryRuntime) Test(profile model.ConnectionProfile) error {
	d, ok := driver.Lookup(profile.Kind)
	if !ok {
		return fmt.Errorf("%w: %s", driver.ErrUnknownKind, profile.Kind)
	}
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout(profile))
	defer cancel()

	conn, err := d.Open(ctx, profile)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return conn.Ping(ctx)
}

func (r *registryRuntime) CloseAll() {
	r.registry.CloseAll()
}
