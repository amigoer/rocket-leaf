import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  DEFAULT_ALERT_RULES,
  DESTINATION_RULES,
  KINDS_NEEDING_DESTINATIONS,
  KINDS_WITH_OWN_RULES,
  loadAlertRules,
  rulesFor,
  saveAlertRules,
  type AlertRulePrefs,
} from "./alertRules";

const KEY = "mq-studio:alert-rules";

describe("alertRules storage", () => {
  const store = new Map<string, string>();

  beforeEach(() => {
    store.clear();
    vi.stubGlobal("window", globalThis);
    vi.stubGlobal("localStorage", {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => {
        store.set(k, v);
      },
      removeItem: (k: string) => {
        store.delete(k);
      },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns defaults when empty", () => {
    const rules = loadAlertRules();
    expect(rules).toEqual(DEFAULT_ALERT_RULES);
  });

  it("persists and reloads toggles", () => {
    const next: AlertRulePrefs = {
      ...DEFAULT_ALERT_RULES,
      groupLag: false,
      dlqGrowth: false,
    };
    saveAlertRules(next);
    expect(loadAlertRules().groupLag).toBe(false);
    expect(loadAlertRules().dlqGrowth).toBe(false);
    expect(loadAlertRules().brokerOffline).toBe(true);
    expect(store.get(KEY)).toBeTruthy();
  });

  it("merges partial stored prefs with defaults", () => {
    store.set(KEY, JSON.stringify({ groupLag: false }));
    const rules = loadAlertRules();
    expect(rules.groupLag).toBe(false);
    expect(rules.brokerOffline).toBe(true);
  });
});

/*
 * The sweep and the rules have to agree about which families need a
 * destination listing.
 *
 * Nothing in the language ties them: useAlertCenter decides what to fetch and
 * alertRules decides what can fire, and a family added to the second without
 * the first gets an alerts page that is armed and cannot fire. That is exactly
 * what shipped for ActiveMQ - three destination rules, no destinations swept -
 * and it was found by opening the app against a broker holding 120 undrained
 * messages and seeing "no active alerts".
 */
describe("the rules that read destinations", () => {
  it("are swept for every family that can raise one", () => {
    const needsThem = new Set<string>(KINDS_NEEDING_DESTINATIONS);
    const reads = new Set<string>(DESTINATION_RULES);

    // Only the families that brought their own rules. One on the fallback
    // runs RocketMQ's, which read RocketMQ's facts the RocketMQ way.
    const missing: string[] = [];
    for (const kind of KINDS_WITH_OWN_RULES) {
      const rules = rulesFor(kind);
      if (rules.some((rule) => reads.has(rule)) && !needsThem.has(kind)) {
        missing.push(kind);
      }
    }
    expect(missing).toEqual([]);
  });

  /*
   * And the other way: a family paying for a listing no rule reads is a
   * request per sweep, per connection, per minute, for nothing.
   */
  it("are not swept for a family whose rules never read them", () => {
    const wasted: string[] = [];
    for (const kind of KINDS_NEEDING_DESTINATIONS) {
      const rules = rulesFor(kind);
      if (!rules.some((rule) => (DESTINATION_RULES as readonly string[]).includes(rule))) {
        wasted.push(kind);
      }
    }
    expect(wasted).toEqual([]);
  });
});
