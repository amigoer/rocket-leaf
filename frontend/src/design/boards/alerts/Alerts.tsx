import { useState } from "react";
import { useTranslation } from "react-i18next";
import { BellOff, Settings as SettingsIcon } from "lucide-react";
import { Page, PageBody, PageHeader, RefreshButton, Toolbar } from "@/design/shell";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { MiniStat, Panel, SectionLabel, Segmented, Status } from "@/components";
import { Notice } from "@/design/boards/BoardState";
import { useAlerts, type AlertEntry } from "@/hooks/useAlerts";
import { rulesFor, type AlertRuleKey } from "@/lib/alertRules";
import { useConnectionScope } from "@/mq/ConnectionScope";
import type { AlertSeverity } from "@/lib/alertDerive";
import type { StatusTone } from "@/components";

const TABS = ["active", "rules"] as const;
type Tab = (typeof TABS)[number];

const SEVERITY_TONE: Record<AlertSeverity, StatusTone> = {
  crit: "err",
  warn: "warn",
  info: "ok",
};

/** What a rule fires as, so its row reads at the weight it will arrive at. */
const RULE_SEVERITY: Record<AlertRuleKey, AlertSeverity> = {
  brokerOffline: "crit",
  // A blocked Pulsar subscription is an outage for whoever reads it: the
  // broker has stopped delivering, and no consumer will catch up.
  subscriptionBlocked: "crit",
  groupOffline: "crit",
  groupLag: "warn",
  diskUsage: "warn",
  dlqGrowth: "info",
  resourceAlarm: "crit",
  nodePartition: "crit",
  memoryUsage: "warn",
  queueNoConsumer: "crit",
  queueBacklog: "warn",
  flowControl: "warn",
  // Under-replicated is a warning and the other two are outages: the topic is
  // still working in the first case and is not in the other two.
  partitionUnderReplicated: "warn",
  partitionOffline: "crit",
  partitionLeaderless: "crit",
  // A NATS stream with no leader is neither readable nor writable; one with a
  // replica behind is still serving and is only unprotected. A server that
  // has dropped clients is a warning because the counter never resets - it
  // says this happened, not that it is happening.
  streamNoLeader: "crit",
  streamUnderReplicated: "warn",
  // A pause is deliberate, so it is a warning rather than an outage: someone
  // asked for this and the messages are still there. What makes it worth
  // raising at all is that it is invisible everywhere else - publishing keeps
  // working and every other figure keeps looking healthy.
  deliveryPaused: "warn",
  // Both of Pub/Sub's are warnings for the same reason: each is a state
  // somebody arrived at deliberately - a topic created ahead of its
  // subscription, a topic deleted with readers still on it - that stops being
  // a plan and starts being a leak if it is left. Neither is an outage while
  // it lasts, and neither shows up anywhere else at all.
  topicUnsubscribed: "warn",
  subscriptionOrphaned: "warn",
  // Service Bus's two, warnings for the same reason again: each is a state
  // somebody arrived at on purpose - the $Default rule deleted, an entity
  // switched off for a migration - that stops being a plan and starts being a
  // leak if it is left. Neither is visible anywhere else in the app.
  subscriptionUnroutable: "warn",
  entityDisabled: "warn",
  slowConsumer: "warn",
};

/**
 * The 告警 page: this connection's alerts, and the rules behind them.
 *
 * A view over the notification centre rather than a second store. The bell
 * above already polls every open connection and holds the records; a page that
 * derived its own would answer differently from the bell the moment a
 * threshold moved.
 *
 * It shows what is firing now. Recovered records are the bell's business —
 * they are how it draws a row that has cleared, and a page listing them would
 * be a history nobody asked this page for.
 */
export function Alerts({ onOpenSettings }: { onOpenSettings?: () => void }) {
  const { t } = useTranslation();
  const { alerts, rules, toggleRule, refresh, loading, hasOnline, lagThreshold, diskThreshold } =
    useAlerts();
  const [tab, setTab] = useState<Tab>("active");

  return (
    <Page>
      <PageHeader
        title={t("alerts.title")}
        subtitle={
          hasOnline ? t("alerts.subtitle", { count: alerts.length }) : t("alerts.subtitleNoConn")
        }
        actions={
          <RefreshButton
            refreshing={loading}
            online={hasOnline}
            onClick={() => void refresh()}
          />
        }
      />

      <Toolbar>
        <Segmented
          value={tab}
          onChange={setTab}
          options={TABS.map((key) => ({ value: key, label: t(`alerts.tabs.${key}`) }))}
        />
      </Toolbar>

      <PageBody>
        {tab === "active" ? (
          <ActiveAlerts alerts={alerts} hasOnline={hasOnline} />
        ) : (
          <Rules
            lagThreshold={lagThreshold}
            diskThreshold={diskThreshold}
            rules={rules}
            onToggle={toggleRule}
            onOpenSettings={onOpenSettings}
          />
        )}
      </PageBody>
    </Page>
  );
}

function ActiveAlerts({
  alerts,
  hasOnline,
}: {
  alerts: readonly AlertEntry[];
  hasOnline: boolean;
}) {
  const { t } = useTranslation();

  if (!hasOnline) {
    return <Notice title={t("board.state.offline")}>{t("board.state.offlineHint")}</Notice>;
  }
  if (alerts.length === 0) {
    return (
      <Notice icon={<BellOff size={22} aria-hidden />} title={t("alerts.active.empty")} />
    );
  }

  return (
    <Panel className="overflow-hidden">
      {alerts.map((alert, index) => (
        <div
          key={alert.key}
          className="flex items-start gap-2.5 px-4 py-3"
          style={{ borderTop: index > 0 ? "1px solid var(--c-border)" : undefined }}
        >
          <Status tone={SEVERITY_TONE[alert.severity]} style={{ fontSize: "10px" }}>
            {t(`alerts.level.${alert.severity}`)}
          </Status>
          <div className="min-w-0 flex-1">
            <div className="text-[12.5px] leading-snug font-medium">{alert.title}</div>
            <div className="mono3 mt-0.5 text-[11.5px] leading-snug text-(--c-mono-dim)">
              {alert.desc}
            </div>
          </div>
          {alert.since != null && (
            <span className="mono3 flex-none text-[10.5px] text-(--c-muted)">
              {t("alerts.active.since", { time: alert.since })}
            </span>
          )}
        </div>
      ))}
    </Panel>
  );
}

function Rules({
  lagThreshold,
  diskThreshold,
  rules,
  onToggle,
  onOpenSettings,
}: {
  lagThreshold: number;
  diskThreshold: number;
  rules: Record<AlertRuleKey, boolean>;
  onToggle: (key: AlertRuleKey) => void;
  onOpenSettings?: () => void;
}) {
  const { t } = useTranslation();
  /* Only the rules this family can raise. A switch for "consumer group has no
     instances" against RabbitMQ would be a switch for something that cannot
     happen. */
  const { kind } = useConnectionScope();
  const keys = rulesFor(kind);

  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-2 gap-2">
        <MiniStat
          label={t("alerts.rules.lagThreshold")}
          value={
            lagThreshold <= 0
              ? t("alerts.rules.thresholdOff")
              : t("alerts.rules.lagThresholdValue", { n: lagThreshold.toLocaleString() })
          }
        />
        <MiniStat
          label={t("alerts.rules.diskThreshold")}
          value={
            diskThreshold <= 0
              ? t("alerts.rules.thresholdOff")
              : t("alerts.rules.diskThresholdValue", { n: diskThreshold })
          }
        />
      </div>

      <Panel className="flex flex-col gap-0 px-4 py-1">
        {keys.map((key, index) => (
          <div
            key={key}
            className="flex items-center gap-3 py-2.5"
            style={{ borderTop: index > 0 ? "1px solid var(--c-border)" : undefined }}
          >
            <Status tone={SEVERITY_TONE[RULE_SEVERITY[key]]} style={{ fontSize: "10px" }}>
              {t(`alerts.level.${RULE_SEVERITY[key]}`)}
            </Status>
            <span className="flex-1 text-[12.5px]">{t(`alerts.rule.${key}`)}</span>
            <Switch
              checked={rules[key]}
              onCheckedChange={() => onToggle(key)}
              aria-label={t(`alerts.rule.${key}`)}
            />
          </div>
        ))}
      </Panel>

      <div className="flex items-start gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <SectionLabel>{t("alerts.rules.title")}</SectionLabel>
          <p className="m-0 text-xs leading-relaxed text-(--c-muted)">
            {t("alerts.rules.desc")}
          </p>
          <p className="m-0 text-[11px] leading-relaxed text-(--c-muted-2)">
            {t("alerts.rules.localOnlyNote")}
          </p>
        </div>
        {onOpenSettings != null && (
          <Button variant="outline" size="sm" className="flex-none" onClick={onOpenSettings}>
            <SettingsIcon size={13} aria-hidden />
            {t("alerts.rules.openSettings")}
          </Button>
        )}
      </div>
    </div>
  );
}
