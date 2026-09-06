import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Boxes, Check, ChevronDown, LoaderCircle } from "lucide-react";
import { Capability } from "@bindings/model/models";
import { listScopes, type Scope } from "@/api/connection";
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Skeleton } from "@/components/ui/skeleton";
import { useCapabilities } from "@/mq/capabilities";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { cn, formatErrorMessage } from "@/lib/utils";
import { scopeKeys, scopeOptions } from "./scopeOptions";

/** What the list is doing, so an empty popover can say why it is empty. */
type Listing =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "ready"; scopes: Scope[] }
  | { kind: "failed"; message: string };

/*
 * Three rows of placeholder while the listing is out.
 *
 * Not decoration: the read walks every topic and every broker's groups, and a
 * one-line "loading" grew into a four-row list when it landed, so the popover
 * opened at one height and jumped to another. Standing in for the rows at
 * their real height means the panel arrives the size it will stay.
 */
function LoadingRows({ label }: { label: string }) {
  return (
    <div className="px-2 py-1.5" role="status" aria-label={label}>
      {["58%", "44%", "51%"].map((width) => (
        <div key={width} className="flex flex-col gap-1.5 px-2 py-2">
          <Skeleton className="h-3.5" style={{ width }} />
          <Skeleton className="h-2.5 w-[35%]" />
        </div>
      ))}
    </div>
  );
}

/** One row: the name over what carries it, so every row is one height. */
function ScopeRow({
  name,
  detail,
  selected,
  onSelect,
  value,
}: {
  name: string;
  detail: string;
  selected: boolean;
  onSelect: () => void;
  /** cmdk's own key for the row, which is not always the name. */
  value: string;
}) {
  return (
    <CommandItem value={value} onSelect={onSelect} className="gap-2 px-2 py-2">
      <span className="flex min-w-0 flex-col gap-0.5">
        <span className="truncate">{name}</span>
        <span className="truncate text-[11px] text-muted-foreground">{detail}</span>
      </span>
      <Check className={cn("ml-auto", selected ? "opacity-100" : "opacity-0")} />
    </CommandItem>
  );
}

/**
 * The sidebar's namespace switcher.
 *
 * A RocketMQ namespace scopes the whole connection rather than one page - it
 * is a prefix the client puts on every topic and group it names - so this
 * belongs in the window chrome and not in a board's toolbar. Switching it is
 * therefore a reconnect: the profile is re-pointed and dialled again, and the
 * choice is stored, so the tab reopens where it was left.
 *
 * The options are read lazily, when the popover opens. They cost a walk over
 * every topic and every broker's consumer groups, which is not a price to pay
 * on each tab switch for a list most people will never open.
 */
export function ScopeSwitcher({
  scope,
  switching = false,
  onSwitch,
}: {
  /** What the connection is scoped to now; empty means the whole cluster. */
  scope: string;
  /** True while a switch is in flight, which is a reconnect. */
  switching?: boolean;
  onSwitch: (next: string) => void;
}) {
  const { t } = useTranslation();
  const capabilities = useCapabilities();
  const { id: connID, kind, online } = useConnectionScope();
  // The family's own wording where it has one, the shared line otherwise.
  const st = (key: string, values?: Record<string, unknown>) => t(scopeKeys(kind, key), values);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [listing, setListing] = useState<Listing>({ kind: "idle" });

  const load = useCallback(() => {
    setListing({ kind: "loading" });
    listScopes(connID)
      .then((scopes) => setListing({ kind: "ready", scopes }))
      .catch((error) => setListing({ kind: "failed", message: formatErrorMessage(error) }));
  }, [connID]);

  // A switch redials, so what was listed came off the previous client. Reading
  // it again on the next open is cheaper than holding a stale list.
  useEffect(() => {
    setListing({ kind: "idle" });
  }, [connID, scope]);

  if (!capabilities.has(Capability.CapConnectionScope)) return null;

  const label = scope === "" ? st("unscoped") : scope;
  const { matched, hidden, typed } = scopeOptions(
    listing.kind === "ready" ? listing.scopes : [],
    query,
  );
  // The one case with nothing to offer and nothing being typed: a cluster
  // where no name carries a namespace at all.
  const barren = listing.kind === "ready" && matched.length === 0 && query.trim() === "";

  const pick = (next: string) => {
    setOpen(false);
    setQuery("");
    if (next !== scope) onSwitch(next);
  };

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next && listing.kind === "idle") load();
        if (!next) setQuery("");
      }}
    >
      <PopoverTrigger asChild>
        <button
          type="button"
          className="ni side3-scope"
          aria-label={st("label", { scope: label })}
          title={st("label", { scope: label })}
          disabled={!online || switching}
        >
          <span className="nic">
            {switching ? (
              <LoaderCircle className="mqs-turning" size={16} aria-hidden />
            ) : (
              // Boxes rather than Layers, which is already the topics entry
              // three rows below: in the collapsed rail the two would be the
              // same glyph twice. It is what the vhosts page uses, so a
              // namespace looks the same wherever the app draws one.
              <Boxes size={16} aria-hidden />
            )}
          </span>
          <span className="nil side3-scope-name">{label}</span>
          {/* The one thing saying this opens rather than navigates. It turns
              with the panel and is dropped in the rail, where there is no
              room for it and the glyph is the whole button. */}
          <ChevronDown size={13} className="side3-scope-caret" aria-hidden />
        </button>
      </PopoverTrigger>
      {/*
       * Dropped from the trigger rather than pushed out to its side. Sideways
       * it landed square on the board being re-scoped - the header and the
       * first rows of the very table the switch is about to change - while
       * downwards it covers the page list, which nobody is reading while they
       * pick a namespace.
       */}
      <PopoverContent align="start" side="bottom" sideOffset={6} className="w-80 p-0">
        <Command shouldFilter={false}>
          <CommandInput
            placeholder={st("search")}
            value={query}
            onValueChange={setQuery}
          />
          <CommandList className="max-h-[min(60vh,22rem)]">
            <CommandGroup>
              {/* Never a discovered name: the unscoped connection reads the
                  whole cluster rather than the topics that happen to carry no
                  prefix. It heads the unfiltered list and leaves once there is
                  a query, because a query is somebody hunting for a name and
                  this is not one. */}
              {query.trim() === "" && (
                <ScopeRow
                  value=" unscoped"
                  name={st("unscoped")}
                  detail={st("unscopedHint")}
                  selected={scope === ""}
                  onSelect={() => pick("")}
                />
              )}
              {matched.map((entry) => (
                <ScopeRow
                  key={entry.name}
                  value={entry.name}
                  name={entry.name}
                  detail={st("counts", {
                    destinations: entry.destinations,
                    subscriptions: entry.subscriptions,
                  })}
                  selected={entry.name === scope}
                  onSelect={() => pick(entry.name)}
                />
              ))}
            </CommandGroup>

            {listing.kind === "loading" && <LoadingRows label={st("loading")} />}
            {listing.kind === "failed" && (
              <div className="px-4 py-3 text-xs leading-relaxed text-(--c-err-text)">
                {listing.message}
              </div>
            )}
            {/* A plain row rather than CommandEmpty: with shouldFilter off,
                cmdk counts the rows we render, and the unscoped one is always
                there - so its empty state never fires. */}
            {barren && (
              <div className="px-4 py-3 text-xs text-muted-foreground">
                {st("empty")}
              </div>
            )}

            {typed !== "" && (
              <>
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem
                    value={` typed:${typed}`}
                    onSelect={() => pick(typed)}
                    className="px-2 py-2"
                  >
                    <span className="truncate">{st("useTyped", { scope: typed })}</span>
                  </CommandItem>
                </CommandGroup>
              </>
            )}
            {hidden > 0 && (
              <div className="px-2 py-1.5 text-center text-xs text-muted-foreground">
                {st("more", { count: hidden })}
              </div>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
