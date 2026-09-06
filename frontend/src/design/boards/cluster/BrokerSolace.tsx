import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import { KV, Panel, PanelHeader, SectionLabel, Status } from "@/components";
import { BoardState } from "@/design/boards/BoardState";
import { useSolaceBroker } from "@/hooks/solace/useSolaceCluster";
import { broker as readBroker, msgVpnServing } from "@/mq/solace/cluster";
import { formatBytes, formatCount } from "@/lib/format";

const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

function percent(value: number | null): string {
  return value == null ? DASH : `${value}%`;
}

/**
 * The Solace broker.
 *
 * One row, and there will never be more. A Solace deployment scales by adding
 * brokers that mesh with each other - each reached on its own SEMP address, so
 * each is its own connection here - and a redundancy pair is not two nodes at
 * all: the two appliances share one virtual router and only the active half
 * answers anything. A list would be a list of one however it were assembled,
 * so this is a panel rather than a table.
 *
 * The spool is the figure this page exists for, and it is drawn as a
 * percentage with both raw numbers beside it on purpose. The broker reports
 * the usage in bytes and the cap in megabytes on the same object, which is a
 * factor of a million between two fields whose names differ by three letters -
 * so the numbers are shown in the units they came in and the percentage is the
 * one the driver already scaled.
 *
 * Two caps rather than one. Every Message VPN has a share, and the broker has
 * a ceiling that all the shares are taken out of; a VPN at 40% of its own quota
 * on a broker that is full behaves exactly like one that is full.
 */
export function BrokerSolace() {
  const { t } = useTranslation();
  const state = useSolaceBroker();

  const broker = useMemo(() => {
    const first = (state.data ?? [])[0];
    return first != null ? readBroker(first) : null;
  }, [state.data]);

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.solace.cluster")}
        subtitle={broker?.address ?? ""}
        actions={
          <RefreshButton
            refreshing={state.refreshing}
            online={state.online}
            onClick={() => void state.refresh()}
          />
        }
      />
      <BoardState state={state}>
        <PageBody>
          {broker == null ? (
            <div style={{ padding: "24px", fontSize: "11.5px", color: "var(--c-muted)" }}>
              {t("board.solace.cluster.nothing")}
            </div>
          ) : (
            <Panel style={{ maxWidth: "560px" }}>
              <PanelHeader
                title={broker.address}
                action={
                  <Status tone={broker.online ? "ok" : "warn"}>
                    {broker.online
                      ? t("board.solace.cluster.online")
                      : t("board.solace.cluster.degraded")}
                  </Status>
                }
              />
              <div style={{ padding: "10px 12px", display: "grid", gap: "8px" }}>
                <SectionLabel>{t("board.solace.cluster.section.broker")}</SectionLabel>
                <KV
                  rows={[
                    [t("board.solace.cluster.version"), broker.version ?? DASH],
                    [
                      t("board.solace.cluster.redundancy"),
                      broker.redundancyEnabled
                        ? t("board.solace.cluster.redundancyOn")
                        : t("board.solace.cluster.redundancyOff"),
                    ],
                    [t("board.solace.cluster.rateIn"), count(broker.rateIn)],
                    [t("board.solace.cluster.rateOut"), count(broker.rateOut)],
                    [
                      t("board.solace.cluster.brokerSpoolMax"),
                      broker.brokerSpoolMaxMb == null
                        ? DASH
                        : formatBytes(broker.brokerSpoolMaxMb * 1024 * 1024),
                    ],
                  ]}
                />

                <SectionLabel>{t("board.solace.cluster.section.msgVpn")}</SectionLabel>
                <KV
                  rows={[
                    [t("board.solace.cluster.msgVpn"), broker.msgVpn],
                    [t("board.solace.cluster.msgVpnState"), broker.msgVpnState ?? DASH],
                    [t("board.solace.cluster.spoolUsed"), percent(broker.spoolPercent)],
                    [
                      t("board.solace.cluster.spoolBytes"),
                      broker.spoolUsedBytes == null ? DASH : formatBytes(broker.spoolUsedBytes),
                    ],
                    [
                      t("board.solace.cluster.spoolMax"),
                      broker.spoolMaxMb == null
                        ? DASH
                        : formatBytes(broker.spoolMaxMb * 1024 * 1024),
                    ],
                    [t("board.solace.cluster.spoolMessages"), count(broker.spoolMsgCount)],
                    [t("board.solace.cluster.maxConnections"), count(broker.maxConnections)],
                  ]}
                />

                {!msgVpnServing(broker.msgVpnState) && (
                  <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                    {t("board.solace.cluster.notServing", { state: broker.msgVpnState ?? DASH })}
                  </div>
                )}
                <div style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                  {t("board.solace.cluster.note")}
                </div>
              </div>
            </Panel>
          )}
        </PageBody>
      </BoardState>
    </Page>
  );
}
