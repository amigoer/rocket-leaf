import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { formatErrorMessage } from "@/lib/utils";
import { nameProblem, submittableName } from "@/mq/azureservicebus/names";
import type { AzureServiceBusEntityInput } from "@/api/azureservicebus";
import type { EntityKind, ServiceBusEntity } from "@/mq/azureservicebus/entities";

/** The bounds Service Bus enforces, so the form can say them before the API. */
const LOCK_MIN_SEC = 5;
const LOCK_MAX_SEC = 300;
const DELIVERY_MIN = 1;
const DELIVERY_MAX = 2000;

interface Draft {
  name: string;
  kind: EntityKind;
  /** Zero means "leave it alone", which is also the service's own default. */
  lockDurationSec: number;
  maxDeliveryCount: number;
  ttlSec: number;
  autoDeleteOnIdleSec: number;
  deadLetterOnExpiry: boolean;
  requiresSession: boolean;
  requiresDuplicateDetection: boolean;
  forwardTo: string;
  forwardDeadLettersTo: string;
}

function draftOf(editing: ServiceBusEntity | null): Draft {
  if (editing == null) {
    return {
      name: "",
      kind: "queue",
      lockDurationSec: 0,
      maxDeliveryCount: 0,
      ttlSec: 0,
      autoDeleteOnIdleSec: 0,
      deadLetterOnExpiry: false,
      requiresSession: false,
      requiresDuplicateDetection: false,
      forwardTo: "",
      forwardDeadLettersTo: "",
    };
  }
  return {
    name: editing.name,
    kind: editing.kind,
    lockDurationSec: editing.lockDurationSec ?? 0,
    maxDeliveryCount: editing.maxDeliveryCount ?? 0,
    // The service spells "never" as a number of seconds bigger than a century,
    // and echoing that back into the form would send it as a real timespan.
    ttlSec: forever(editing.ttlSec) ? 0 : (editing.ttlSec ?? 0),
    autoDeleteOnIdleSec: forever(editing.autoDeleteOnIdleSec)
      ? 0
      : (editing.autoDeleteOnIdleSec ?? 0),
    deadLetterOnExpiry: editing.deadLetterOnExpiry,
    requiresSession: editing.requiresSession,
    requiresDuplicateDetection: editing.requiresDuplicateDetection,
    forwardTo: editing.forwardTo ?? "",
    forwardDeadLettersTo: editing.forwardDeadLettersTo ?? "",
  };
}

function forever(seconds: number | null): boolean {
  return seconds != null && seconds > 100 * 365 * 86400;
}

function numberField(raw: string): number {
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) || parsed < 0 ? 0 : parsed;
}

/**
 * Create or edit one queue or topic.
 *
 * The kind is chosen once. A queue and a topic are different objects addressed
 * at the same path, so changing it would mean deleting one and creating the
 * other - which loses whatever it held, and on a topic every subscription too.
 * The driver refuses rather than doing that, and the control is disabled on an
 * edit rather than left to fail.
 *
 * A topic hides the delivery half of the form, and that is the family rather
 * than a simplification: a topic holds no messages, so the lock duration, the
 * delivery limit and the dead-letter switch all belong to its subscriptions.
 *
 * Sessions and duplicate detection are shown only on a create for the same
 * reason the driver only sends them there: the service refuses both in an
 * update, and a control that always fails is worse than one that is not drawn.
 *
 * A name cannot change either. That is the service's rule rather than a design
 * choice: an entity is addressed by its name and there is no rename call.
 */
export function EntityDialogAzureServiceBus({
  open,
  editing,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  /** Null creates; an entity edits it. */
  editing: ServiceBusEntity | null;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: AzureServiceBusEntityInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Draft>(() => draftOf(editing));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(draftOf(editing));
      setError(null);
    }
  }, [open, editing]);

  const set = <K extends keyof Draft>(key: K, next: Draft[K]) =>
    setDraft((current) => ({ ...current, [key]: next }));

  const problem = draft.name === "" ? null : nameProblem(draft.name);
  const submitted = submittableName(draft.name);
  const isQueue = draft.kind === "queue";
  const lockOutOfRange =
    isQueue &&
    draft.lockDurationSec !== 0 &&
    (draft.lockDurationSec < LOCK_MIN_SEC || draft.lockDurationSec > LOCK_MAX_SEC);
  const deliveryOutOfRange =
    isQueue &&
    draft.maxDeliveryCount !== 0 &&
    (draft.maxDeliveryCount < DELIVERY_MIN || draft.maxDeliveryCount > DELIVERY_MAX);

  const submit = async () => {
    if (submitted == null || lockOutOfRange || deliveryOutOfRange || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit({
        name: submitted,
        kind: draft.kind,
        // A topic carries none of the delivery half; sending it would be
        // asking the service to configure something it does not have.
        lockDurationSec: isQueue ? draft.lockDurationSec : 0,
        maxDeliveryCount: isQueue ? draft.maxDeliveryCount : 0,
        deadLetterOnExpiry: isQueue && draft.deadLetterOnExpiry,
        ttlSec: draft.ttlSec,
        autoDeleteOnIdleSec: draft.autoDeleteOnIdleSec,
        maxSizeMb: 0,
        requiresSession: isQueue && draft.requiresSession,
        requiresDuplicateDetection: draft.requiresDuplicateDetection,
        partitioned: false,
        forwardTo: isQueue ? draft.forwardTo.trim() : "",
        forwardDeadLettersTo: isQueue ? draft.forwardDeadLettersTo.trim() : "",
      });
      onOpenChange(false);
    } catch (submitError) {
      setError(formatErrorMessage(submitError));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t(
              editing == null
                ? "board.azure-servicebus.entities.createTitle"
                : "board.azure-servicebus.entities.editTitle",
              { name: editing?.name ?? "" },
            )}
          </DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="sb-entity-name">
              {t("board.azure-servicebus.entities.name")}
            </FieldLabel>
            <Input
              id="sb-entity-name"
              value={draft.name}
              disabled={editing != null}
              placeholder="orders"
              onChange={(event) => set("name", event.target.value)}
            />
            <FieldDescription>
              {problem === "reserved"
                ? t("board.azure-servicebus.entities.nameProblem.reserved")
                : problem === "separator"
                  ? t("board.azure-servicebus.entities.nameProblem.separator")
                  : problem === "tooLong"
                    ? t("board.azure-servicebus.entities.nameProblem.tooLong")
                    : problem === "charset"
                      ? t("board.azure-servicebus.entities.nameProblem.charset")
                      : problem === "empty"
                        ? t("board.azure-servicebus.entities.nameProblem.empty")
                        : t("board.azure-servicebus.entities.nameHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel>{t("board.azure-servicebus.entities.kind")}</FieldLabel>
            <div style={{ display: "flex", gap: "6px" }}>
              <Button
                type="button"
                size="sm"
                variant={isQueue ? "default" : "outline"}
                disabled={editing != null}
                onClick={() => set("kind", "queue")}
              >
                {t("board.azure-servicebus.entities.kindQueue")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant={isQueue ? "outline" : "default"}
                disabled={editing != null}
                onClick={() => set("kind", "topic")}
              >
                {t("board.azure-servicebus.entities.kindTopic")}
              </Button>
            </div>
            <FieldDescription>
              {t("board.azure-servicebus.entities.kindHint")}
            </FieldDescription>
          </Field>

          {isQueue && (
            <>
              <Field>
                <FieldLabel htmlFor="sb-entity-lock">
                  {t("board.azure-servicebus.entities.lockDuration")}
                </FieldLabel>
                <Input
                  id="sb-entity-lock"
                  type="number"
                  value={String(draft.lockDurationSec)}
                  onChange={(event) => set("lockDurationSec", numberField(event.target.value))}
                />
                <FieldDescription style={lockOutOfRange ? { color: "var(--c-danger)" } : undefined}>
                  {lockOutOfRange
                    ? t("board.azure-servicebus.entities.lockRange")
                    : t("board.azure-servicebus.entities.lockHint")}
                </FieldDescription>
              </Field>

              <Field>
                <FieldLabel htmlFor="sb-entity-delivery">
                  {t("board.azure-servicebus.entities.maxDelivery")}
                </FieldLabel>
                <Input
                  id="sb-entity-delivery"
                  type="number"
                  value={String(draft.maxDeliveryCount)}
                  onChange={(event) => set("maxDeliveryCount", numberField(event.target.value))}
                />
                <FieldDescription
                  style={deliveryOutOfRange ? { color: "var(--c-danger)" } : undefined}
                >
                  {deliveryOutOfRange
                    ? t("board.azure-servicebus.entities.maxDeliveryRange")
                    : t("board.azure-servicebus.entities.maxDeliveryHint")}
                </FieldDescription>
              </Field>

              <Field orientation="horizontal">
                <Switch
                  id="sb-entity-dlq-expiry"
                  checked={draft.deadLetterOnExpiry}
                  onCheckedChange={(next) => set("deadLetterOnExpiry", next)}
                />
                <FieldLabel htmlFor="sb-entity-dlq-expiry">
                  {t("board.azure-servicebus.entities.deadLetterOnExpiry")}
                </FieldLabel>
              </Field>

              <Field>
                <FieldLabel htmlFor="sb-entity-forward-dl">
                  {t("board.azure-servicebus.entities.forwardDeadLettersTo")}
                </FieldLabel>
                <Input
                  id="sb-entity-forward-dl"
                  value={draft.forwardDeadLettersTo}
                  placeholder="failed-orders"
                  onChange={(event) => set("forwardDeadLettersTo", event.target.value)}
                />
                <FieldDescription>
                  {t("board.azure-servicebus.entities.forwardDeadLettersToHint")}
                </FieldDescription>
              </Field>
            </>
          )}

          <Field>
            <FieldLabel htmlFor="sb-entity-ttl">
              {t("board.azure-servicebus.entities.ttl")}
            </FieldLabel>
            <Input
              id="sb-entity-ttl"
              type="number"
              value={String(draft.ttlSec)}
              onChange={(event) => set("ttlSec", numberField(event.target.value))}
            />
            <FieldDescription>{t("board.azure-servicebus.entities.ttlHint")}</FieldDescription>
          </Field>

          {editing == null && (
            <>
              {isQueue && (
                <Field orientation="horizontal">
                  <Switch
                    id="sb-entity-sessions"
                    checked={draft.requiresSession}
                    onCheckedChange={(next) => set("requiresSession", next)}
                  />
                  <FieldLabel htmlFor="sb-entity-sessions">
                    {t("board.azure-servicebus.entities.sessions")}
                  </FieldLabel>
                </Field>
              )}
              <Field orientation="horizontal">
                <Switch
                  id="sb-entity-dedup"
                  checked={draft.requiresDuplicateDetection}
                  onCheckedChange={(next) => set("requiresDuplicateDetection", next)}
                />
                <FieldLabel htmlFor="sb-entity-dedup">
                  {t("board.azure-servicebus.entities.duplicateDetection")}
                </FieldLabel>
              </Field>
              <FieldDescription>
                {t("board.azure-servicebus.entities.fixedAtCreationHint")}
              </FieldDescription>
            </>
          )}

          {error != null && (
            <FieldDescription style={{ color: "var(--c-danger)" }}>{error}</FieldDescription>
          )}
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={submitted == null || lockOutOfRange || deliveryOutOfRange || busy}
            onClick={() => void submit()}
          >
            {busy && <Spinner />}
            {t(editing == null ? "common.create" : "common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
