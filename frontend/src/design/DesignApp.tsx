import { useCallback, useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";
import {
  AppShell,
  CommandPalette,
  ConnectionTabs,
  Sidebar,
  scopeKeys,
  TabStatusBar,
  TitleBar,
} from "@/design/shell";
import type { Connection } from "@/design/data/connections";
import { labelOf, pagesOf, type PageId } from "@/design/data/protocols";
import { renderBoard, type BoardFocus } from "@/design/registry";
import {
  onTrayNavigate,
  openExternal,
  reportShellSession,
  type TrayDestination,
} from "@/api/platform";
import { useUIScale } from "@/hooks/useUIScale";
import { useUpdater } from "@/hooks/useUpdater";
import { latencyLabel, useConnectionProfiles } from "@/hooks/useConnectionProfiles";
import { ConnectionScopeProvider } from "@/mq/ConnectionScope";
import { CapabilitiesProvider } from "@/mq/capabilities";
import { useConfirm, useToast } from "@/components";
import { exportAllConfigToFile, importAllConfigFromFile } from "@/api/settings";
import {
  probeConnection as probeDraft,
  type ConnectionDraft,
  type CredentialsMode,
} from "@/api/connection";
import { readSession, restoreSession, writeSession } from "@/design/data/session";
import { ConnectionsList } from "@/design/boards/connections/ConnectionsList";
import { ConnectionsEmpty } from "@/design/boards/connections/ConnectionsEmpty";
import { NewConnectionDialog } from "@/design/boards/connections/NewConnectionDialog";
import { UpdateDialog } from "@/design/shell/UpdateDialog";
import { Settings, type SectionId } from "@/design/boards/settings/Settings";

/** Global views sit beside the connection tabs rather than inside one. */
type View =
  | { kind: "tab" }
  | { kind: "connections" }
  | { kind: "settings"; section?: SectionId };

const GITHUB_URL = "https://github.com/amigoer/mq-studio";

/**
 * The repository link goes through Go rather than window.open: the webview has
 * no browser to open a tab in, and SystemService.OpenExternal is where the
 * host allow-list lives. It rejects anything off github.com, which is nothing
 * a user can act on, so a failure stays quiet.
 */
const openGithub = () => void openExternal(GITHUB_URL).catch(() => {});

/**
 * The design canvas realised: window → connection tab → page (5c). Each tab
 * keeps its own page selection, which is why `pageByTab` is keyed by tab and
 * not stored globally.
 */
export function DesignApp(): JSX.Element {
  const { t, i18n } = useTranslation();
  const {
    connections,
    profiles,
    loading: connectionsLoading,
    loadFailed: connectionsLoadFailed,
    pending,
    errors,
    reload,
    remove,
    makeDefault,
    connect,
    disconnect,
    test,
    switchScope,
    create,
    update,
  } = useConnectionProfiles();
  const toast = useToast();
  const confirm = useConfirm();
  // Read once: the stored session is the window's opening state, and reading it
  // again on a later render would fight whatever the user has done since. It is
  // filtered against the profiles below, once they have loaded.
  const [session] = useState(() => readSession());
  const [openTabs, setOpenTabs] = useState<string[]>([]);
  const [activeTab, setActiveTab] = useState<string | null>(null);
  const [pageByTab, setPageByTab] = useState<Record<string, PageId>>({});
  /*
   * Where a cross-page jump landed. `seq` is what makes the board remount even
   * when the jump does not change the page, so arriving twice on the same page
   * with a different topic is honoured rather than swallowed.
   */
  const [focus, setFocus] = useState<{ seq: number; value: BoardFocus } | null>(null);
  const [restored, setRestored] = useState(false);
  const [view, setView] = useState<View>({ kind: "connections" });
  const [previousView, setPreviousView] = useState<View>({ kind: "tab" });

  // Window chrome, not per-tab state: see the note on Sidebar.
  const [navCollapsed, setNavCollapsed] = useState(session.navCollapsed ?? false);

  // The marker stays lit for as long as an update is pending: it is a state,
  // not a notification, and it clears itself when the update is taken or
  // skipped.
  const {
    available: updateAvailable,
    checking: updateChecking,
    check: checkUpdate,
    openDialog: openUpdate,
  } = useUpdater();

  // Applied to the document, not to this tree: every board is drawn in absolute
  // px and the whole document is zoomed to the chosen size.
  const { setting: scaleSetting, fontSize, setSetting: setScale } = useUIScale();

  // One dialog serves both gestures: `editing` is the profile id being edited,
  // or null for a new connection.
  const [dialog, setDialog] = useState<{ editing: number | null } | null>(null);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [paletteQuery, setPaletteQuery] = useState("");

  /*
   * The session names profiles, so it can only be restored once they have
   * loaded: a tab whose profile is gone must not reopen. Reopening on the
   * connection list would restore the tabs and then hide them, so a session
   * that names a tab reopens on it.
   */
  useEffect(() => {
    // A list that never arrived is not a list without these profiles in it.
    // Restoring on one would drop every tab, and the write below would then
    // persist that emptiness over the session it failed to read.
    if (connectionsLoading || connectionsLoadFailed || restored) return;
    setRestored(true);
    const opening = restoreSession(session, connections.map((c) => c.key));
    setPageByTab(opening.pageByTab);
    if (opening.openTabs.length === 0) return;
    setOpenTabs(opening.openTabs);
    setActiveTab(opening.activeTab);
    setView({ kind: "tab" });
  }, [connections, connectionsLoading, connectionsLoadFailed, restored, session]);

  useEffect(() => {
    if (!restored) return;
    writeSession({ openTabs, activeTab, pageByTab, navCollapsed });
  }, [activeTab, navCollapsed, openTabs, pageByTab, restored]);

  const connection = connections.find((c) => c.key === activeTab) ?? null;
  const protocol = connection?.protocol ?? null;
  const page: PageId = (activeTab != null ? pageByTab[activeTab] : undefined) ?? "overview";

  const goto = useCallback(
    (next: View) => {
      setView((current) => {
        if (current.kind === "tab") setPreviousView(current);
        return next;
      });
    },
    [],
  );

  // ⌘K / Ctrl+K opens the palette from anywhere (9d); ⌘B collapses the sidebar.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!e.metaKey && !e.ctrlKey) return;
      const key = e.key.toLowerCase();
      if (key === "k") {
        e.preventDefault();
        setPaletteOpen((open) => !open);
      } else if (key === "b") {
        e.preventDefault();
        setNavCollapsed((collapsed) => !collapsed);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const openTab = useCallback((key: string) => {
    setOpenTabs((tabs) => (tabs.includes(key) ? tabs : [...tabs, key]));
    setActiveTab(key);
    setView({ kind: "tab" });
  }, []);

  const closeTab = (key: string) => {
    setOpenTabs((tabs) => {
      const next = tabs.filter((t) => t !== key);
      setActiveTab((current) => (current === key ? (next[0] ?? null) : current));
      if (next.length === 0) setView({ kind: "connections" });
      return next;
    });
  };

  /*
   * The tray menu's destination. A page only exists inside a tab, so a request
   * naming a connection opens or raises that tab first, and one that names
   * none lands in whichever tab is in front. With no tab to land in there is
   * nothing to show but the connection list.
   */
  const trayNavigate = useCallback(
    (to: TrayDestination) => {
      if (to.page === "settings") {
        goto({ kind: "settings" });
        return;
      }
      const key = to.connection !== "" ? to.connection : activeTab;
      if (to.page === "connections" || key == null) {
        goto({ kind: "connections" });
        return;
      }
      openTab(key);
      // The connections submenu sends no page: raising a tab must not cost the
      // user the page it was left on.
      if (to.page !== "") setPageByTab((byTab) => ({ ...byTab, [key]: to.page as PageId }));
    },
    [activeTab, goto, openTab],
  );

  useEffect(() => onTrayNavigate(trayNavigate), [trayNavigate]);

  /*
   * The other half of that conversation: the tray menu offers the active tab's
   * sidebar, and can only do so because the labels are reported to it. They
   * are resolved here, so a language change re-reports rather than leaving Go
   * holding the previous language's menu.
   */
  useEffect(() => {
    const pages =
      protocol == null
        ? []
        : pagesOf(protocol).map((id) => ({ id, label: t(labelOf(protocol, id)) }));
    void reportShellSession(activeTab ?? "", page, pages).catch(() => {
      // Off Wails there is no tray to tell.
    });
  }, [activeTab, i18n.language, page, protocol, t]);

  const deleteConnection = async (connection: Connection) => {
    const confirmed = await confirm({
      title: t("page.connections.deleteTitle"),
      description: t("page.connections.deleteDesc", { name: connection.name }),
      confirmLabel: t("page.connections.deleteAction"),
      danger: true,
    });
    if (!confirmed) return;
    try {
      await remove(connection.id);
      setOpenTabs((tabs) => tabs.filter((t) => t !== connection.key));
      setActiveTab((current) => (current === connection.key ? null : current));
      toast.success(t("page.connections.deleted", { name: connection.name }));
    } catch (error) {
      toast.error(t("page.connections.deleteFailed"), { description: String(error) });
    }
  };

  const connectConnection = async (connection: Connection) => {
    const result = await connect(connection.id);
    if (result.ok) {
      toast.success(t("page.connections.connected", { name: connection.name }));
    } else {
      toast.error(t("page.connections.connectFailed", { name: connection.name }), {
        description: result.error,
      });
    }
    return result.ok;
  };

  const disconnectConnection = async (connection: Connection) => {
    const result = await disconnect(connection.id);
    if (!result.ok) {
      toast.error(t("page.connections.disconnectFailed"), { description: result.error });
      return;
    }
    // A tab whose connection is closed would show pages that cannot answer.
    setOpenTabs((tabs) => tabs.filter((key) => key !== connection.key));
    setActiveTab((current) => (current === connection.key ? null : current));
    toast.success(t("page.connections.disconnected", { name: connection.name }));
  };

  const probeConnection = async (connection: Connection) => {
    const result = await test(connection.id);
    if (!result.ok) {
      toast.error(t("page.connections.probeFailed"), { description: result.error });
      return;
    }
    toast.success(t("page.connections.probeOk", { latency: latencyLabel(result.latencyMs) }));
  };

  const saveConnection = async (
    draft: ConnectionDraft,
    credentialsMode: CredentialsMode,
  ) => {
    const editingID = dialog?.editing ?? null;
    const saved =
      editingID != null
        ? await update(editingID, draft, credentialsMode)
        : await create(draft);
    toast.success(t("page.connections.saved", { name: saved.name }));
    // "Save and connect" is what the button says on a new connection, and a
    // profile nobody has dialled is not much use on its own.
    if (editingID == null) await connect(saved.id);
  };

  /*
   * A namespace switch is a reconnect: the profile is re-pointed and dialled
   * again, so the pages behind it are reading a different set of names as soon
   * as it lands. Leaving the tab where it is - same page, same connection - is
   * the whole point of putting the switch in the chrome.
   */
  const switchConnectionScope = async (connection: Connection, next: string) => {
    if ((connection.scope ?? "") === next) return;
    const result = await switchScope(connection.id, next);
    if (!result.ok) {
      toast.error(t(scopeKeys(connection.kind, "switchFailed")), { description: result.error });
      return;
    }
    toast.success(
      next === ""
        ? t(scopeKeys(connection.kind, "switchedUnscoped"))
        : t(scopeKeys(connection.kind, "switched"), { scope: next }),
    );
  };

  const promoteConnection = async (connection: Connection) => {
    try {
      await makeDefault(connection.id);
      toast.success(t("page.connections.defaultSet", { name: connection.name }));
    } catch (error) {
      toast.error(t("page.connections.defaultFailed"), { description: String(error) });
    }
  };

  const exportConfig = async () => {
    try {
      const path = await exportAllConfigToFile();
      if (path == null) return;
      toast.success(t("page.settings.data.exported"), { description: path });
    } catch (error) {
      toast.error(t("page.settings.data.exportFailed"), { description: String(error) });
    }
  };

  const importConfig = async () => {
    const confirmed = await confirm({
      title: t("page.settings.data.import"),
      description: t("page.settings.data.importDesc"),
      confirmLabel: t("page.settings.data.importConfirm"),
      danger: true,
    });
    if (!confirmed) return;
    try {
      const path = await importAllConfigFromFile();
      if (path == null) return;
      await reload();
      toast.success(t("page.settings.data.imported"), { description: path });
    } catch (error) {
      toast.error(t("page.settings.data.importFailed"), { description: String(error) });
    }
  };

  const selectPage = (next: PageId, nextFocus?: BoardFocus) => {
    if (activeTab == null) return;
    // Reaching a page from the sidebar carries no focus, and must clear the
    // one a previous jump left behind.
    setFocus(nextFocus == null ? null : { seq: (focus?.seq ?? 0) + 1, value: nextFocus });
    setPageByTab((byTab) => ({ ...byTab, [activeTab]: next }));
  };

  const onConnection = view.kind === "tab" && protocol != null;
  // The connection list is the home page, and a tab with no connection behind
  // it falls back to the same list, so the mark reads as selected there too.
  const atHome = view.kind === "connections" || (view.kind === "tab" && !onConnection);

  const sidebar =
    onConnection && protocol != null && connection != null ? (
      <Sidebar
        protocol={protocol}
        active={pagesOf(protocol).includes(page) ? page : "overview"}
        collapsed={navCollapsed}
        scope={connection.scope ?? ""}
        switchingScope={pending[connection.id] === "connecting"}
        onSelect={selectPage}
        onToggle={() => setNavCollapsed((collapsed) => !collapsed)}
        onSwitchScope={(next) => void switchConnectionScope(connection, next)}
      />
    ) : undefined;

  const editingProfile =
    dialog?.editing != null ? profiles.find((p) => p.id === dialog.editing) : undefined;

  /*
   * Everything under the title bar reads one connection: the tab in front. The
   * sidebar is inside the scope too, because which entries it draws is a
   * question about that connection's capabilities.
   */
  const scopedProfile =
    activeTab != null ? profiles.find((p) => String(p.id) === activeTab) : undefined;

  /*
   * Opening a tab on a profile nobody has dialled would land on pages that
   * cannot answer, so the tab opens and the connection follows. It opens even
   * when the dial fails: the tab is where the failure is legible.
   */
  const openConnectionTab = (connection: Connection) => {
    openTab(connection.key);
    if (connection.status !== "online" && pending[connection.id] == null) {
      void connectConnection(connection);
    }
  };

  const connectionsBoard = (
    <ConnectionsList
      connections={connections}
      pending={pending}
      errors={errors}
      onNewConnection={() => setDialog({ editing: null })}
      onOpenTab={(key) => {
        const connection = connections.find((c) => c.key === key);
        if (connection != null) openConnectionTab(connection);
      }}
      onDelete={(connection) => void deleteConnection(connection)}
      onSetDefault={(connection) => void promoteConnection(connection)}
      onImport={() => void importConfig()}
      onExport={() => void exportConfig()}
      onConnect={(connection) => void connectConnection(connection)}
      onDisconnect={(connection) => void disconnectConnection(connection)}
      onTest={(connection) => void probeConnection(connection)}
      onEdit={(connection) => setDialog({ editing: connection.id })}
    />
  );

  const content = (() => {
    switch (view.kind) {
      case "connections":
        return connections.length === 0 ? (
          <ConnectionsEmpty
            onNewConnection={() => setDialog({ editing: null })}
            onImport={() => void importConfig()}
          />
        ) : (
          connectionsBoard
        );
      case "settings":
        return (
          <Settings
            onBack={() => setView(previousView)}
            scale={{ setting: scaleSetting, fontSize, onChange: setScale }}
            initialSection={view.section}
          />
        );
      case "tab":
      default:
        if (protocol == null) {
          return connections.length === 0 ? (
            <ConnectionsEmpty
              onNewConnection={() => setDialog({ editing: null })}
              onImport={() => void importConfig()}
            />
          ) : (
            connectionsBoard
          );
        }
        return renderBoard(protocol, pagesOf(protocol).includes(page) ? page : "overview", {
          onOpenAlertSettings: () => goto({ kind: "settings", section: "message" }),
          onOpenPage: selectPage,
          focus: focus?.value,
        });
    }
  })();

  const online = connections.filter((c) => c.status === "online").length;

  /*
   * The page transition. Everything the key names already remounts the column
   * on its own -- a different board is a different component, a different tab
   * is a different connection -- so keying on it costs nothing the switch was
   * not already paying, and gives the change one fade to arrive on.
   *
   * The scope is the exception: it changes under a board that would otherwise
   * stay mounted, and every board reads on mount and then on its own 30-second
   * timer. Without it, switching namespace left the previous one's topics on
   * screen for up to half a minute with nothing saying so.
   */
  const viewKey = [
    view.kind,
    view.kind === "settings" ? "" : (activeTab ?? ""),
    onConnection ? page : "",
    onConnection ? (connection?.scope ?? "") : "",
    onConnection ? String(focus?.seq ?? 0) : "",
  ].join(":");

  const column = (
    <div key={viewKey} className="mqs-view" style={{ flex: 1, display: "flex", minWidth: 0 }}>
      {content}
    </div>
  );

  const shell = (
    <AppShell
      titleBar={
        <TitleBar
          homeActive={atHome}
          dimmed={connections.length === 0}
          checking={updateChecking}
          updateAvailable={updateAvailable}
          onHome={() => goto({ kind: "connections" })}
          onSearch={() => setPaletteOpen(true)}
          /* A pending release opens where it can be taken; with nothing
             pending the same button is what goes and looks. */
          onUpdate={() => (updateAvailable != null ? openUpdate() : void checkUpdate())}
          onGithub={openGithub}
          onOpenAlertSettings={() => goto({ kind: "settings", section: "message" })}
          onOpenConnection={(id) => {
            openTab(String(id));
            goto({ kind: "tab" });
          }}
          onSettings={() => goto({ kind: "settings" })}
          tabs={
            <ConnectionTabs
              tabs={openTabs}
              connections={connections}
              /*
               * 3a / 3g keep the connection tab highlighted behind a global
               * view; the home page is the one that does not, because it is
               * itself a tab and two tabs cannot both be selected.
               */
              active={atHome ? null : activeTab}
              onSelect={openTab}
              onClose={closeTab}
              onAdd={() => {
                goto({ kind: "connections" });
                setDialog({ editing: null });
              }}
            />
          }
        />
      }
      sidebar={sidebar}
      overlays={
        <>
          <NewConnectionDialog
            open={dialog != null}
            onClose={() => setDialog(null)}
            editing={editingProfile}
            onSubmit={saveConnection}
            onProbe={async (draft, credentialsMode) => {
              const started = performance.now();
              await probeDraft(draft, dialog?.editing ?? 0, credentialsMode);
              return performance.now() - started;
            }}
          />
          <CommandPalette
            open={paletteOpen}
            query={paletteQuery}
            connections={connections}
            protocol={protocol}
            onQueryChange={setPaletteQuery}
            onOpenConnection={openTab}
            onOpenPage={selectPage}
            onNewConnection={() => {
              goto({ kind: "connections" });
              setDialog({ editing: null });
            }}
            onOpenSettings={() => goto({ kind: "settings" })}
            onCheckUpdate={() => void checkUpdate()}
            onClose={() => setPaletteOpen(false)}
          />
          {/* Opens over whatever is on screen: the release is announced by the
              title bar and by a toast, and both hand off to this. */}
          <UpdateDialog />
        </>
      }
    >
      {onConnection && connection != null ? (
        <div style={{ flex: 1, display: "flex", flexDirection: "column", minWidth: 0 }}>
          <div style={{ flex: 1, display: "flex", minHeight: 0 }}>{column}</div>
          <TabStatusBar
            connection={connection.name}
            address={connection.address}
            scope={connection.scope}
            status={connection.status}
            latency={connection.latency}
            tabCount={openTabs.length}
            onlineCount={online}
          />
        </div>
      ) : (
        column
      )}
    </AppShell>
  );

  return (
    <ConnectionScopeProvider profile={scopedProfile}>
      <CapabilitiesProvider>{shell}</CapabilitiesProvider>
    </ConnectionScopeProvider>
  );
}
