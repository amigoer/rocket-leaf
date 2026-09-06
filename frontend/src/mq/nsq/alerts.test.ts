import { describe, expect, it } from "vitest";
import { deriveNsqAlerts } from "./alerts";
import en from "@/i18n/locales/en.json";
import zh from "@/i18n/locales/zh.json";
import { DEFAULT_ALERT_RULES, rulesFor } from "@/lib/alertRules";
import { NO_FACTS, type AlertFacts } from "@/lib/alertDerive";
import { MQKind } from "@bindings/model/models";
import type { Destination, Subscription } from "@/api/models";

const THRESHOLDS = { lag: 100, disk: 80 };

/** Only the slice of a locale bundle this test reads. */
type Bundle = { alerts: { detail: Record<string, string> } };

function channel(
  topic: string,
  name: string,
  backlog: number,
  clients: number,
  paused = false,
): Subscription {
  return {
    id: 0,
    ref: { namespace: topic, name },
    status: clients > 0 ? "online" : "offline",
    members: clients,
    destinations: 1,
    backlog,
    rateOut: -1,
    lastUpdated: "",
    attributes: { topic, paused: String(paused) },
  } as unknown as Subscription;
}

function topic(
  name: string,
  topicDepth: number,
  channels: string[],
  paused = false,
): Destination {
  return {
    id: 0,
    ref: { namespace: "", name },
    partitions: -1,
    subscribers: channels.length,
    depth: topicDepth,
    rateIn: -1,
    rateOut: -1,
    lastUpdated: "",
    attributes: {
      topicDepth: String(topicDepth),
      channelDepth: "0",
      channels: channels.join(","),
      paused: String(paused),
    },
  } as unknown as Destination;
}

function facts(partial: Partial<AlertFacts>): AlertFacts {
  return { ...NO_FACTS, ...partial };
}

describe("nsq alerts", () => {
  it("raises a channel with a backlog and nothing attached", () => {
    const alerts = deriveNsqAlerts(
      facts({ consumerGroups: [channel("orders", "analytics", 40, 0)] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts.map((alert) => alert.ruleKey)).toEqual(["groupOffline"]);
    expect(alerts[0]?.params.group).toBe("orders/analytics");
  });

  /*
   * A channel holding messages for a consumer that is not connected is what a
   * channel is for. Firing on every idle channel would make the page useless
   * on a healthy cluster.
   */
  it("says nothing about an idle channel with no backlog", () => {
    const alerts = deriveNsqAlerts(
      facts({ consumerGroups: [channel("orders", "analytics", 0, 0)] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts).toEqual([]);
  });

  it("raises a backlog past the threshold on a channel that is being read", () => {
    const alerts = deriveNsqAlerts(
      facts({ consumerGroups: [channel("orders", "analytics", 250, 2)] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts.map((alert) => alert.ruleKey)).toEqual(["groupLag"]);
  });

  // A channel nobody is draining is already the stronger statement about the
  // same backlog. Firing both would put one problem on the page twice.
  it("does not also raise a lag on a channel with no consumer", () => {
    const alerts = deriveNsqAlerts(
      facts({ consumerGroups: [channel("orders", "analytics", 5000, 0)] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts.map((alert) => alert.ruleKey)).toEqual(["groupOffline"]);
  });

  /*
   * The pause outranks both, and that is the whole reason the rule exists.
   * The consumers are connected and asking for work; nothing is wrong with
   * them, and neither backlog rule would describe what is actually happening.
   */
  it("raises the pause rather than the backlog it is causing", () => {
    const alerts = deriveNsqAlerts(
      facts({ consumerGroups: [channel("orders", "analytics", 5000, 3, true)] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts.map((alert) => alert.ruleKey)).toEqual(["deliveryPaused"]);
    expect(alerts[0]?.params.target).toBe("orders/analytics");
  });

  it("says nothing about a paused channel that is holding nothing", () => {
    const alerts = deriveNsqAlerts(
      facts({ consumerGroups: [channel("orders", "analytics", 0, 1, true)] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts).toEqual([]);
  });

  /*
   * The one state where messages are lost by neglect rather than by failure.
   * nsqd copies a message into the channels that exist when it arrives, so a
   * topic nothing has ever subscribed to fills its own queue and is read by
   * nobody.
   */
  it("raises a topic holding messages that reached no channel", () => {
    const alerts = deriveNsqAlerts(
      facts({ destinations: [topic("orders", 300, [])] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts.map((alert) => alert.ruleKey)).toEqual(["queueNoConsumer"]);
    expect(alerts[0]?.params.queue).toBe("orders");
  });

  it("says nothing about a topic whose messages have reached a channel", () => {
    const alerts = deriveNsqAlerts(
      facts({ destinations: [topic("orders", 0, ["analytics"])] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts).toEqual([]);
  });

  it("raises a paused topic rather than calling it unsubscribed", () => {
    const alerts = deriveNsqAlerts(
      facts({ destinations: [topic("orders", 300, ["analytics"], true)] }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts.map((alert) => alert.ruleKey)).toEqual(["deliveryPaused"]);
  });

  it("raises nothing when the rules are switched off", () => {
    const off = Object.fromEntries(
      Object.keys(DEFAULT_ALERT_RULES).map((key) => [key, false]),
    ) as typeof DEFAULT_ALERT_RULES;
    const alerts = deriveNsqAlerts(
      facts({
        consumerGroups: [channel("orders", "analytics", 5000, 0, true)],
        destinations: [topic("orders", 300, [])],
      }),
      off,
      THRESHOLDS,
    );
    expect(alerts).toEqual([]);
  });

  /*
   * The rules offered in the settings list and the rules this family can
   * produce have to be the same set. A switch for something that cannot fire
   * is a control that does nothing; a rule that fires with no switch cannot be
   * turned off.
   */
  it("offers exactly the rules it can raise", () => {
    expect([...rulesFor(MQKind.KindNSQ)].sort()).toEqual(
      ["deliveryPaused", "groupLag", "groupOffline", "queueNoConsumer"].sort(),
    );
  });

  // The detail line is interpolated at render, so a parameter the sentence
  // does not name is a placeholder printed to the user.
  it("fills every placeholder its detail lines name, in both languages", () => {
    const alerts = deriveNsqAlerts(
      facts({
        consumerGroups: [
          channel("orders", "analytics", 5000, 0),
          channel("orders", "archive", 250, 2),
          channel("audit", "analytics", 40, 1, true),
        ],
        destinations: [topic("events", 300, [])],
      }),
      DEFAULT_ALERT_RULES,
      THRESHOLDS,
    );
    expect(alerts).toHaveLength(4);

    for (const bundle of [en as unknown as Bundle, zh as unknown as Bundle]) {
      for (const alert of alerts) {
        const sentence = bundle.alerts.detail[alert.ruleKey];
        expect(sentence, alert.ruleKey).toBeTruthy();
        for (const match of (sentence ?? "").matchAll(/\{\{(\w+)\}\}/g)) {
          const name = match[1] ?? "";
          expect(alert.params, `${alert.ruleKey}.${name}`).toHaveProperty(name);
        }
      }
    }
  });
});
