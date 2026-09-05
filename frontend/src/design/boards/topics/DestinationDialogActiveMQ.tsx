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
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { formatErrorMessage } from "@/lib/utils";
import type { DestinationKind } from "@/mq/activemq/destinations";

/**
 * The name a submission carries, or null when there is nothing to submit.
 *
 * Exported because it is the only rule this form has and the only thing it can
 * get wrong: a name with surrounding whitespace is a different destination,
 * and both brokers would accept it without a word.
 */
export function destinationNameOf(typed: string): string | null {
  const name = typed.trim();
  return name === "" ? null : name;
}

/**
 * A name and a kind, and nothing else.
 *
 * Every other family's create dialog collects settings because its
 * destinations have some at creation time. A JMS destination has none that a
 * client can set: what would be configured lives in the broker's own file, as
 * a policy entry on Classic and an address setting on Artemis, both matched by
 * name after the fact. Offering a field the broker would ignore is the mistake
 * this dialog exists to not make.
 *
 * The kind cannot be inferred and is not guessed. A queue and a topic can
 * share a name - they are different objects in different trees on Classic, and
 * one address with two routing types on Artemis - so asking is the only way to
 * make the one that was meant.
 */
export function DestinationDialogActiveMQ({
  open,
  onOpenChange,
  onCreate,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (name: string, kind: DestinationKind) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<DestinationKind>("queue");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName("");
      setKind("queue");
      setError(null);
    }
  }, [open]);

  const submitted = destinationNameOf(name);

  const submit = async () => {
    if (submitted == null || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onCreate(submitted, kind);
      onOpenChange(false);
    } catch (createError) {
      setError(formatErrorMessage(createError));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("board.activemq.destinations.createTitle")}</DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel>{t("board.activemq.destinations.name")}</FieldLabel>
            <Input
              className="mono3"
              value={name}
              placeholder="ORDERS.inbound"
              onChange={(event) => setName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") void submit();
              }}
            />
          </Field>
          <Field>
            <FieldLabel>{t("board.activemq.destinations.kindColumn")}</FieldLabel>
            <ToggleGroup
              type="single"
              variant="outline"
              size="sm"
              value={kind}
              onValueChange={(next: string) => setKind((next as DestinationKind) || "queue")}
            >
              <ToggleGroupItem value="queue" className="text-[11px]">
                {t("board.activemq.destinations.kind.queue")}
              </ToggleGroupItem>
              <ToggleGroupItem value="topic" className="text-[11px]">
                {t("board.activemq.destinations.kind.topic")}
              </ToggleGroupItem>
            </ToggleGroup>
            <FieldDescription>
              {t("board.activemq.destinations.kindHint")}
            </FieldDescription>
          </Field>
          {error != null && (
            <FieldDescription style={{ color: "var(--c-danger)" }}>{error}</FieldDescription>
          )}
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button disabled={submitted == null || busy} onClick={() => void submit()}>
            {busy && <Spinner />}
            {t("common.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Where to move a destination's messages.
 *
 * One field, because ActiveMQ has one thing to say: a JMS move puts the
 * message in the named destination and there is no exchange or routing key in
 * between for it to take. The canonical move request carries both, and this
 * driver reads the routing key as the target's name.
 */
export function MoveDialogActiveMQ({
  open,
  onOpenChange,
  from,
  onMove,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  from: string;
  onMove: (to: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [to, setTo] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setTo("");
      setError(null);
    }
  }, [open]);

  const submitted = destinationNameOf(to);

  const submit = async () => {
    if (submitted == null || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onMove(submitted);
      onOpenChange(false);
    } catch (moveError) {
      setError(formatErrorMessage(moveError));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("board.activemq.destinations.moveTitle", { name: from })}</DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel>{t("board.activemq.destinations.moveTarget")}</FieldLabel>
            <Input
              className="mono3"
              value={to}
              onChange={(event) => setTo(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") void submit();
              }}
            />
            <FieldDescription>{t("board.activemq.destinations.moveHint")}</FieldDescription>
          </Field>
          {error != null && (
            <FieldDescription style={{ color: "var(--c-danger)" }}>{error}</FieldDescription>
          )}
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button disabled={submitted == null || busy} onClick={() => void submit()}>
            {busy && <Spinner />}
            {t("board.activemq.destinations.moveAction")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
