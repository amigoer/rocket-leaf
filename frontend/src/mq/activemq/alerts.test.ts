import { describe, expect, it } from "vitest";
import { deriveActiveMQAlerts } from "./alerts";
import { DEFAULT_ALERT_RULES } from "@/lib/alertRules";
import { NO_FACTS, type AlertFacts } from "@/lib/alertDerive";
import type { Destination, Node, Subscription } from "@/api/models";

const THRESHOLDS = { lag: 100, disk: 80 };

function queue(name: string, depth: number, consumers: number, dead = false): Destination {
  return {
    id: 0,
    ref: { namespace: "", name },
    partitions: -1,
    subscribers: consumers,
    depth,
    rateIn: -1,
    rateOut: -1,
    lastUpdated: "",
    attributes: {
      product: "artemis",
      kind: "queue",
      consumerCount: String(consumers),
      ...(dead ? { isDeadLetter: "true" } : {}),
    },
  } as unknown as Destination;
}

function facts(partial: Partial<AlertFacts>): AlertFacts {
  return { ...NO_FACTS, ...partial };
}

describe("activemq alerts", () => {
  it("raises a queue nobody is draining", () => {
    const alerts = deriveActiveMQAlerts(
      facts({ destinations: [queue("ORDERS", 40, 0)] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts.map((alert) => alert.ruleKey)).toEqual(["queueNoConsumer"]);
  });

  /*
   * A queue nobody is draining is already the stronger statement about the
   * same queue. Firing both would put one problem on the page twice.
   */
  it("does not also raise a backlog on the same queue", () => {
    const alerts = deriveActiveMQAlerts(
      facts({ destinations: [queue("ORDERS", 5000, 0)] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts).toHaveLength(1);
    expect(alerts[0]?.ruleKey).toBe("queueNoConsumer");
  });

  it("raises a backlog on a queue that is being drained too slowly", () => {
    const alerts = deriveActiveMQAlerts(
      facts({ destinations: [queue("ORDERS", 5000, 2)] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts.map((alert) => alert.ruleKey)).toEqual(["queueBacklog"]);
  });

  /*
   * Every dead-letter queue has a backlog and no consumer - that is what it
   * is. Letting the ordinary queue rules see one would make a healthy
   * deployment fire two alerts about a destination doing its job.
   */
  it("measures a dead-letter queue with its own rule and no other", () => {
    const alerts = deriveActiveMQAlerts(
      facts({ destinations: [queue("DLQ", 12, 0, true)] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts.map((alert) => alert.ruleKey)).toEqual(["dlqGrowth"]);
  });

  /*
   * The absence that matters most. A durable subscription with nothing
   * attached is the state durability exists for, so a rule for it would fire
   * on every healthy broker.
   */
  it("says nothing about a durable subscription with nothing attached", () => {
    const detached = {
      id: 0,
      ref: { namespace: "EVENTS", name: "EVENTS.analytics" },
      status: "offline",
      members: 0,
      destinations: 1,
      backlog: 900,
      rateOut: -1,
      lastUpdated: "",
      attributes: { product: "artemis", subscriptionName: "analytics", active: "false" },
    } as unknown as Subscription;

    const alerts = deriveActiveMQAlerts(
      facts({ consumerGroups: [detached] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts).toEqual([]);
  });

  /*
   * A bridged broker answers on its own console and reports nothing here, so
   * reading its figures would be reading zeros - and a zero disk reading is
   * not an alert either way.
   */
  it("does not read a bridged broker's figures", () => {
    const bridge = {
      id: 0,
      name: "to-other",
      address: "",
      cluster: "bridge",
      version: "",
      status: "unknown",
      rateIn: -1,
      rateOut: -1,
      diskUsage: -1,
      lastSeen: "",
      attributes: { product: "artemis", bridge: "true" },
    } as unknown as Node;

    const alerts = deriveActiveMQAlerts(
      facts({ nodes: [bridge] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts).toEqual([]);
  });

  it("raises a store past the threshold, and calls a nearly full one critical", () => {
    const broker = (disk: number) =>
      ({
        id: 0,
        name: "0.0.0.0",
        address: "http://127.0.0.1:8161",
        cluster: "",
        version: "2.44.0",
        status: "online",
        rateIn: -1,
        rateOut: -1,
        diskUsage: disk,
        lastSeen: "",
        attributes: { product: "artemis" },
      }) as unknown as Node;

    expect(
      deriveActiveMQAlerts(facts({ nodes: [broker(85)] }), DEFAULT_ALERT_RULES, THRESHOLDS)[0],
    ).toMatchObject({ ruleKey: "diskUsage", severity: "warn" });
    expect(
      deriveActiveMQAlerts(facts({ nodes: [broker(97)] }), DEFAULT_ALERT_RULES, THRESHOLDS)[0],
    ).toMatchObject({ ruleKey: "diskUsage", severity: "crit" });
  });
});
