package solace

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * The Message VPN as a scope.
 *
 * This is the family's own answer to a question every driver here has had to
 * settle: is the partition a connection sits in part of its address, or a
 * selector the shell offers? IBM MQ answered "address" for its queue manager,
 * and the reasoning is in that driver's package comment - a queue manager is a
 * separate process with its own storage, its own log and its own listener, and
 * a second one is a second broker.
 *
 * A Message VPN is none of those. It is a partition inside one running broker:
 * the same process, the same message spool, the same disk, the same TCP
 * listeners. The broker answers for itself with no VPN named at all - its
 * version, its spool, its rates - and it enumerates its VPNs on request, which
 * is what the listing below is. Switching one changes a path segment and
 * re-points every board at once, and dials nothing new. That is the whole of
 * what CapConnectionScope is for.
 *
 * The one thing that differs from RocketMQ's namespace, whose shape this
 * borrows, is that a Message VPN is a real object rather than a prefix. So the
 * listing is what the broker holds rather than what its names imply, and it is
 * complete: there is no VPN that exists only because something is named after
 * it. What ValidateScope can still not do is check existence - it takes no
 * context and makes no call - so it enforces the broker's own syntax rule and
 * leaves "there is no such VPN" to the redial, which says so by name.
 */

// maxScopeVPNs bounds how many VPNs are counted for the switcher.
//
// The listing itself is one request; the counts are two more per VPN, and a
// broker hosting hundreds of Message VPNs would turn a popover into several
// hundred round trips. Past this the names are still all offered and the
// counts beside them are left unknown, which is the half worth dropping.
const maxScopeVPNs = 60

/*
 * ListScopes reports the Message VPNs this broker hosts.
 *
 * Deliberately unfiltered by this connection's own VPN: the whole point is to
 * show what it could be switched to.
 *
 * The counts are the collections' own meta.count, for the reason the
 * destination listing gives - SEMP fills it with how many exist rather than
 * how many it returned, so one request asking for a single row answers "how
 * many are there". Destinations are queues and Subscriptions are topic
 * endpoints: those are the two kinds of endpoint a VPN holds, and they are
 * what makes one row in the switcher distinguishable from another.
 */
func (c *Conn) ListScopes(ctx context.Context) ([]*model.Scope, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	names, err := c.listMsgVPNNames(ctx)
	if err != nil {
		return nil, err
	}

	scopes := make([]*model.Scope, 0, len(names))
	for _, name := range names {
		scopes = append(scopes, &model.Scope{
			Name:          name,
			Destinations:  model.UnknownMetric,
			Subscriptions: model.UnknownMetric,
		})
	}
	if len(scopes) > maxScopeVPNs {
		return scopes, nil
	}

	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(countConcurrency)
	for _, scope := range scopes {
		group.Go(func() error {
			queues, err := c.vpnCollectionCount(ctx, scope.Name, "queues")
			if err != nil {
				return err
			}
			scope.Destinations = queues
			return nil
		})
		group.Go(func() error {
			endpoints, err := c.vpnCollectionCount(ctx, scope.Name, "topicEndpoints")
			if err != nil {
				return err
			}
			scope.Subscriptions = endpoints
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	sort.Slice(scopes, func(i, j int) bool { return scopes[i].Name < scopes[j].Name })
	return scopes, nil
}

// vpnCollectionCount is how many entries one of a Message VPN's collections
// holds.
func (c *Conn) vpnCollectionCount(ctx context.Context, vpn, collection string) (int, error) {
	path := monitorAPI + "/msgVpns/" + segment(vpn) + "/" + collection + "?count=1"
	_, meta, err := c.semp.do(ctx, "GET", path, nil)
	if err != nil {
		if notFound(err) {
			return model.UnknownMetric, nil
		}
		return 0, err
	}
	return meta.Count, nil
}

/*
 * ValidateScope reports whether a name could be a Message VPN at all.
 *
 * Syntax only, and that is the port's shape rather than a shortcut: the method
 * takes no context and makes no call, so it cannot ask the broker whether the
 * name exists. What it can do is refuse the two things the broker refuses -
 * the rule below is quoted from the message SEMP answers a bad name with - so
 * a typo with a wildcard in it is stopped at the switcher instead of being
 * stored and failing on the redial.
 *
 * An empty name is valid, and it means something here rather than nothing: a
 * profile that names no Message VPN is resolved at dial time, to the broker's
 * only one or to "default". It is a real, storable, working state, which is
 * what the switcher's unscoped row offers.
 */
func (c *Conn) ValidateScope(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > maxMsgVPNNameLength {
		return fmt.Errorf("a message vpn name is at most %d characters; %q is %d",
			maxMsgVPNNameLength, trimmed, len(trimmed))
	}
	if strings.ContainsAny(trimmed, "*?") {
		return fmt.Errorf("a message vpn name cannot contain * or ? and %q does", trimmed)
	}
	return nil
}

// maxMsgVPNNameLength is the broker's, quoted from the message it refuses a
// longer one with.
const maxMsgVPNNameLength = 32
