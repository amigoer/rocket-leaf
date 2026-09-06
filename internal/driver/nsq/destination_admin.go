package nsq

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

// Creating and removing topics.
//
// Both are cluster-wide, because a topic is not: it exists on the nsqd it was
// created on and nowhere else, so creating it on one daemon would leave a
// producer that connected to another either failing or auto-creating it later
// with none of the channels.

// errNoUpdate is what UpdateDestination reports.
//
// The method exists because DestinationAdmin is one interface; the capability
// is not declared, so nothing in the UI reaches this. See the reason in
// conformance_test.go.
var errNoUpdate = errors.New("an nsq topic has no configuration to change; its name is the whole of it")

// CreateDestination declares a topic on every nsqd in the connection.
func (c *Conn) CreateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	if spec.Ref.Name == "" {
		return errors.New("a topic needs a name")
	}
	_, err := eachNode(ctx, c.nodes, func(ctx context.Context, n node) (struct{}, error) {
		return struct{}{}, c.client.post(ctx, n.address, "/topic/create",
			url.Values{"topic": {spec.Ref.Name}}, nil, nil)
	})
	return err
}

// UpdateDestination is not offered. See errNoUpdate.
func (c *Conn) UpdateDestination(_ context.Context, _ model.DestinationSpec) error {
	return errNoUpdate
}

/*
 * RemoveDestination deletes a topic and everything under it.
 *
 * Two tiers, and both are required for the delete to hold. nsqd forgets the
 * topic; nsqlookupd does not, because a registration is separate from the
 * topic's existence - so a delete that only reached nsqd leaves the name in
 * nsqlookupd's /topics, where nsqadmin still lists it and a consumer looking
 * it up still finds it. Confirmed against 1.3.0: after deleting on nsqd alone,
 * /topics still names the topic and /lookup answers with an empty producer
 * list rather than a 404.
 *
 * nsqd goes first, and the order is not arbitrary. nsqd registers with the
 * discovery tier asynchronously, so a delete that cleared the tier first can
 * be undone by a registration still in flight from the create - which is a
 * delete that silently leaves the object behind, and which this test suite
 * caught. Deleting at nsqd first queues the unregister behind any pending
 * register on the same connection, and the directory sweep afterwards removes
 * whatever the unregister did not.
 *
 * The sweep runs when the daemons agreed - either they deleted it, or none of
 * them was carrying it. A registration whose nsqd is long gone is exactly the
 * state that needs cleaning, and refusing to sweep would make it undeletable;
 * sweeping after a daemon actually failed would be the opposite mistake, and
 * leave a topic that is still there and no longer findable.
 */
func (c *Conn) RemoveDestination(ctx context.Context, ref model.DestinationRef) error {
	if err := c.live(); err != nil {
		return err
	}
	if ref.Name == "" {
		return errors.New("a topic needs a name")
	}

	query := url.Values{"topic": {ref.Name}}
	removed := c.onEveryCarrier(ctx, "/topic/delete", query,
		fmt.Sprintf("no nsqd in this connection was carrying the topic %q", ref.Name))
	if removed != nil && !notCarried(removed) {
		return removed
	}

	if err := c.forgetAtLookupd(ctx, "/topic/delete", query); err != nil {
		return err
	}
	return removed
}

// forgetAtLookupd runs a delete against every configured nsqlookupd.
//
// An object the discovery tier has never heard of is not an error: a topic
// created while the tier was down is registered nowhere, and refusing to
// delete it would leave it undeletable.
//
// The sweep cannot report what it cleaned, and the caller must not infer it.
// nsqlookupd answers /topic/delete with 200 whether or not it had the topic -
// confirmed against 1.3.0 against a name it had never seen - so "the
// registration was stale and is now gone" and "there was nothing anywhere" are
// the same answer. Only /channel/delete tells them apart, and one delete
// behaving differently from the other would be worse than neither doing so.
func (c *Conn) forgetAtLookupd(ctx context.Context, path string, query url.Values) error {
	group, groupCtx := errgroup.WithContext(ctx)
	for _, address := range c.config.lookupd {
		group.Go(func() error {
			err := c.client.post(groupCtx, address, path, query, nil, nil)
			if err != nil && !notFound(err) {
				return fmt.Errorf("%s: %w", hostPort(address), err)
			}
			return nil
		})
	}
	return group.Wait()
}
