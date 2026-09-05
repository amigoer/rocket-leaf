package connection

import "github.com/amigoer/mq-studio/internal/model"

// ClientRuntime is the mutable client registry, isolated from profile
// persistence so lifecycle transactions can be tested deterministically.
//
// The implementation lives in the composition root rather than here: binding
// it in this package is what made connection management import a driver, and
// a profile store has no business knowing which broker family it stores
// profiles for.
//
// It is keyed by profile id, not by endpoint. Two profiles may name the same
// address - the same cluster under different credentials is the ordinary case -
// and an endpoint key made those one connection, so closing either closed both.
//
// The profile handed to Connect and Test is already resolved: this package has
// filled in the timeout and any ACL credentials that fall back to application
// settings, so the runtime dials exactly what it is given.
type ClientRuntime interface {
	Connect(profile model.ConnectionProfile) error
	HasClient(id int) bool
	Remove(id int)
	Test(profile model.ConnectionProfile) error
	CloseAll()
}

// EndpointPolicy answers whether a family must be given an address, from
// the driver's own descriptor. Injected for the same reason ClientRuntime
// is: a profile store must not import a driver.
type EndpointPolicy interface {
	RequiresEndpoints(kind model.MQKind) bool
}
