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
import { SelectField } from "@/components";
import { formatErrorMessage } from "@/lib/utils";
import { queueNameProblem, reservedName, sharesConsumers } from "@/mq/solace/names";
import type { SolaceQueueInput } from "@/api/solace";

/**
 * Exclusive is the broker's own default and is offered second.
 *
 * The order is not cosmetic. Exclusive hands every message to the first
 * consumer that binds and keeps the rest as standbys, which is right for
 * ordered processing and is also exactly what somebody who scaled a service
 * out did not want. Putting the sharing one first makes the choice a choice.
 */
const ACCESS_TYPES = ["non-exclusive", "exclusive"] as const;

/** What a client bound to the queue may do to it, weakest first. */
const PERMISSIONS = ["read-only", "consume", "modify-topic", "delete"] as const;

interface Draft {
  name: string;
  accessType: string;
  permission: string;
  owner: string;
  deadMsgQueue: string;
  maxRedeliveryCount: string;
  maxSpoolUsageMb: string;
}

const EMPTY: Draft = {
  name: "",
  accessType: "non-exclusive",
  permission: "consume",
  owner: "",
  deadMsgQueue: "",
  maxRedeliveryCount: "",
  maxSpoolUsageMb: "",
};

/**
 * Create one queue in the connection's Message VPN.
 *
 * There is no edit here, and that is deliberate rather than unfinished. A
 * queue's settings change underneath whatever has it bound, and the ones worth
 * changing - the access type, the dead message queue, whether ingress is
 * enabled - each have their own consequence for consumers already attached, so
 * this driver reads them and offers no single control that writes them all.
 *
 * The dead message queue field takes a name and does not create one. Solace's
 * default for it is #DEAD_MSG_QUEUE, a name that points at nothing until
 * somebody makes a queue called that - so a queue can be configured to
 * dead-letter and quietly discard instead, which is why the field is empty
 * here rather than pre-filled with the broker's default.
 */
export function QueueDialogSolace({
  open,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: SolaceQueueInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Draft>(EMPTY);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(EMPTY);
      setError(null);
    }
  }, [open]);

  const set = <K extends keyof Draft>(key: K, next: Draft[K]) =>
    setDraft((current) => ({ ...current, [key]: next }));

  const name = draft.name.trim();
  const problem = name === "" ? null : queueNameProblem(name);
  const reserved = name !== "" && reservedName(name);
  const blocked = name === "" || problem != null || reserved;

  const submit = async () => {
    if (blocked || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit({
        name,
        accessType: draft.accessType,
        permission: draft.permission,
        owner: draft.owner.trim(),
        deadMsgQueue: draft.deadMsgQueue.trim(),
        maxRedeliveryCount: Number.parseInt(draft.maxRedeliveryCount, 10) || 0,
        maxSpoolUsageMb: Number.parseInt(draft.maxSpoolUsageMb, 10) || 0,
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
          <DialogTitle>{t("board.solace.queues.createTitle")}</DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="solace-name">{t("board.solace.queues.name")}</FieldLabel>
            <Input
              id="solace-name"
              className="mono3"
              value={draft.name}
              placeholder="orders/eu"
              onChange={(event) => set("name", event.target.value)}
            />
            <FieldDescription>
              {reserved
                ? t("board.solace.queues.nameProblem.reserved")
                : problem != null
                  ? t(`board.solace.queues.nameProblem.${problem}`)
                  : t("board.solace.queues.nameHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="solace-access">{t("board.solace.queues.accessType")}</FieldLabel>
            <SelectField<string>
              value={draft.accessType}
              options={ACCESS_TYPES.map((value) => ({ value, label: value }))}
              onValueChange={(next) => set("accessType", next)}
            />
            <FieldDescription>
              {sharesConsumers(draft.accessType)
                ? t("board.solace.queues.accessShared")
                : t("board.solace.queues.accessExclusive")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="solace-permission">
              {t("board.solace.queues.permission")}
            </FieldLabel>
            <SelectField<string>
              value={draft.permission}
              options={PERMISSIONS.map((value) => ({ value, label: value }))}
              onValueChange={(next) => set("permission", next)}
            />
            <FieldDescription>{t("board.solace.queues.permissionHint")}</FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="solace-dmq">{t("board.solace.queues.deadMsgQueue")}</FieldLabel>
            <Input
              id="solace-dmq"
              className="mono3"
              value={draft.deadMsgQueue}
              onChange={(event) => set("deadMsgQueue", event.target.value)}
            />
            <FieldDescription>{t("board.solace.queues.deadMsgQueueHint")}</FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="solace-redelivery">
              {t("board.solace.queues.maxRedelivery")}
            </FieldLabel>
            <Input
              id="solace-redelivery"
              type="number"
              value={draft.maxRedeliveryCount}
              placeholder="0"
              onChange={(event) => set("maxRedeliveryCount", event.target.value)}
            />
            <FieldDescription>{t("board.solace.queues.maxRedeliveryHint")}</FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="solace-quota">{t("board.solace.queues.maxSpool")}</FieldLabel>
            <Input
              id="solace-quota"
              type="number"
              value={draft.maxSpoolUsageMb}
              placeholder="5000"
              onChange={(event) => set("maxSpoolUsageMb", event.target.value)}
            />
            <FieldDescription>{t("board.solace.queues.maxSpoolHint")}</FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="solace-owner">{t("board.solace.queues.owner")}</FieldLabel>
            <Input
              id="solace-owner"
              value={draft.owner}
              onChange={(event) => set("owner", event.target.value)}
            />
            <FieldDescription>{t("board.solace.queues.ownerHint")}</FieldDescription>
          </Field>

          {error != null && (
            <FieldDescription style={{ color: "var(--c-danger)" }}>{error}</FieldDescription>
          )}
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button disabled={blocked || busy} onClick={() => void submit()}>
            {busy && <Spinner />}
            {t("common.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
