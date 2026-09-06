/**
 * Service Bus's view of the canonical routing shapes.
 *
 * The keys are a contract with internal/driver/azureservicebus/routing.go.
 *
 * A rule is what decides which of a topic's messages reach one subscription,
 * and it is an object rather than a field: it has a name, several may sit on
 * one subscription, and each is a filter plus an optional action that rewrites
 * the message on the way in. That is why this page exists at all - every other
 * family here keeps its filtering on the reader, where it is a setting.
 *
 * It arrives as a canonical Binding because the routing page is shared. The
 * mapping is exact rather than approximate: the source is the topic, the
 * destination is the subscription, the routing key is the filter, and the
 * properties key is the rule's name - which is genuinely the handle, since a
 * rule is deleted by name.
 */
import type { Binding } from "@bindings/model/models";

const ArgFilterType = "filterType";
const ArgExpression = "expression";
const ArgAction = "action";

/** Correlation fields carry this prefix, so they stay apart from the rest. */
const CorrelationPrefix = "correlation.";

export type FilterKind = "sql" | "correlation" | "true" | "false";

/** The rule the service adds to every new subscription. It matches everything. */
export const DEFAULT_RULE = "$Default";

export interface ServiceBusRule {
  /** The rule's name, and what deletes it. */
  name: string;
  topic: string;
  subscription: string;

  kind: FilterKind;
  /** The SQL filter's text, on a sql rule. */
  expression: string | null;
  /** The message fields a correlation rule compares, by equality. */
  correlation: [string, string][];
  /** A SQL statement run on a matching message before it is copied in. */
  action: string | null;

  /** The filter as the routing column shows it, whichever kind it is. */
  summary: string;
}

function argument(binding: Binding, key: string): string | null {
  const value = binding.arguments?.[key];
  return value == null || value === "" ? null : value;
}

export function rule(binding: Binding): ServiceBusRule {
  const raw = argument(binding, ArgFilterType);
  const kind: FilterKind =
    raw === "sql" || raw === "correlation" || raw === "false" ? raw : "true";
  return {
    name: binding.propertiesKey,
    topic: binding.source,
    subscription: binding.destination,
    kind,
    expression: argument(binding, ArgExpression),
    correlation: Object.entries(binding.arguments ?? {})
      .filter(([key]) => key.startsWith(CorrelationPrefix))
      .map(([key, value]) => [key.slice(CorrelationPrefix.length), value] as [string, string])
      .sort(([a], [b]) => a.localeCompare(b)),
    action: argument(binding, ArgAction),
    summary: binding.routingKey,
  };
}

/**
 * Whether this rule lets everything through.
 *
 * Worth its own reader because it is the state a subscription starts in: the
 * $Default rule matches every message, so a subscription that still has it is
 * receiving the whole topic whatever else has been added beside it.
 */
export function matchesEverything(entry: ServiceBusRule): boolean {
  return entry.kind === "true";
}

/** Whether this rule can never match, which is legal and rarely intended. */
export function matchesNothing(entry: ServiceBusRule): boolean {
  return entry.kind === "false";
}

/**
 * Which subscriptions on a topic have no rule at all.
 *
 * The state this page exists to make visible. A subscription with no rules
 * receives nothing, reports itself Active, and shows an empty backlog because
 * nothing can arrive - so it is invisible on every board but this one and the
 * subscriptions list's status column.
 */
export function subscriptionsWithNoRule(
  rules: ServiceBusRule[],
  subscriptions: { topic: string; name: string }[],
): string[] {
  const withRules = new Set(rules.map((entry) => `${entry.topic}/${entry.subscription}`));
  return subscriptions
    .map((row) => `${row.topic}/${row.name}`)
    .filter((path) => !withRules.has(path));
}
