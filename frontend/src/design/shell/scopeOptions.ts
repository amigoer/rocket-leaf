import type { Scope } from "@/api/connection";
import { filterOptions } from "@/lib/optionFilter";

/**
 * What the scope switcher's popover draws for one query.
 *
 * Beside the component rather than inside it because React Fast Refresh gives
 * up on a module that exports both a component and something else: every edit
 * to the switcher reloaded the module outright, which closed the very popover
 * being worked on.
 */
export interface ScopeOptions {
  /** The discovered namespaces that matched, ranked and capped. */
  matched: Scope[];
  /** Matches beyond the render cap, so the popover can say how many. */
  hidden: number;
  /**
   * The typed name, when it has to be offered on its own.
   *
   * A namespace nothing carries yet is invisible to the listing and still
   * perfectly usable - it is a prefix, not an object - so a name the cluster
   * has never seen is offered as itself. One the list already holds is not:
   * it is already a row, and a second one saying the same thing reads as a
   * different destination.
   */
  typed: string;
}

export function scopeOptions(scopes: readonly Scope[], query: string): ScopeOptions {
  const byName = new Map(scopes.map((entry) => [entry.name, entry]));
  const filtered = filterOptions(
    scopes.map((entry) => entry.name),
    query,
  );
  const typed = query.trim();
  return {
    matched: filtered.items.flatMap((name) => byName.get(name) ?? []),
    hidden: filtered.hidden,
    typed: typed !== "" && !byName.has(typed) ? typed : "",
  };
}

/**
 * The i18n keys for one piece of the switcher's copy, family first.
 *
 * The shared wording is RocketMQ's, because RocketMQ was the only family with
 * a switchable scope for a long time: "All namespaces" means the whole cluster
 * with names unchanged, which is exactly what an unscoped RocketMQ connection
 * reads. It is not what an unnamed Solace profile does - that one is resolved
 * to a single Message VPN at dial time - so a family whose scope means
 * something else says so in its own bundle and everything else falls through
 * to the shared line.
 *
 * A list rather than a lookup because i18next takes one and uses the first key
 * that resolves, so a family needs to override only the lines that would be
 * wrong rather than all of them.
 */
export function scopeKeys(kind: string | undefined, key: string): string[] {
  const shared = `shell.scope.${key}`;
  if (kind == null || kind === "") return [shared];
  return [`mq.${kind}.scope.${key}`, shared];
}
