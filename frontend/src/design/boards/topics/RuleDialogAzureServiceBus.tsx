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
import { formatErrorMessage } from "@/lib/utils";
import { nameProblem, submittableName } from "@/mq/azureservicebus/names";
import type { FilterKind } from "@/mq/azureservicebus/rules";
import type { AzureServiceBusRuleInput } from "@/api/azureservicebus";

interface Draft {
  topic: string;
  subscription: string;
  name: string;
  kind: FilterKind;
  expression: string;
  /** The correlation kind's commonest field by far, and the only one drawn. */
  subject: string;
  action: string;
}

function emptyDraft(): Draft {
  return {
    topic: "",
    subscription: "",
    name: "",
    kind: "sql",
    expression: "",
    subject: "",
    action: "",
  };
}

/**
 * Add one rule to a subscription.
 *
 * Create only, and that is the service rather than a shortcut: a rule's filter
 * kind cannot be changed, so editing one is a delete and a create - a SQL rule
 * and a correlation rule are different objects wearing one name.
 *
 * The two kinds are drawn differently because they take different text. A SQL
 * filter is an expression over the message's own fields and the sender's
 * properties; a correlation filter is a set of fields compared by equality,
 * and the subject is the one people actually use, so it is the one drawn.
 *
 * The action is optional on both and is the half of a rule that changes the
 * message rather than selecting it - it runs on a matching message before the
 * copy is placed in the subscription.
 */
export function RuleDialogAzureServiceBus({
  open,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: AzureServiceBusRuleInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(emptyDraft());
      setError(null);
    }
  }, [open]);

  const set = <K extends keyof Draft>(key: K, next: Draft[K]) =>
    setDraft((current) => ({ ...current, [key]: next }));

  const topic = submittableName(draft.topic);
  const subscription = submittableName(draft.subscription, "child");
  const name = submittableName(draft.name, "child");
  const nameIssue = draft.name === "" ? null : nameProblem(draft.name, "child");
  const needsExpression = draft.kind === "sql" && draft.expression.trim() === "";
  const needsSubject = draft.kind === "correlation" && draft.subject.trim() === "";

  const submit = async () => {
    if (
      topic == null ||
      subscription == null ||
      name == null ||
      needsExpression ||
      needsSubject ||
      busy
    ) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await onSubmit({
        topic,
        subscription,
        name,
        kind: draft.kind,
        expression: draft.kind === "sql" ? draft.expression.trim() : "",
        correlation:
          draft.kind === "correlation" ? { subject: draft.subject.trim() } : {},
        action: draft.action.trim(),
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
          <DialogTitle>{t("board.azure-servicebus.rules.createTitle")}</DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="sb-rule-topic">
              {t("board.azure-servicebus.rules.topic")}
            </FieldLabel>
            <Input
              id="sb-rule-topic"
              value={draft.topic}
              placeholder="events"
              onChange={(event) => set("topic", event.target.value)}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="sb-rule-subscription">
              {t("board.azure-servicebus.rules.subscription")}
            </FieldLabel>
            <Input
              id="sb-rule-subscription"
              value={draft.subscription}
              placeholder="orders-worker"
              onChange={(event) => set("subscription", event.target.value)}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="sb-rule-name">
              {t("board.azure-servicebus.rules.name")}
            </FieldLabel>
            <Input
              id="sb-rule-name"
              value={draft.name}
              placeholder="red-only"
              onChange={(event) => set("name", event.target.value)}
            />
            <FieldDescription>
              {nameIssue === "tooLong"
                ? t("board.azure-servicebus.rules.nameProblem.tooLong")
                : nameIssue === "charset"
                  ? t("board.azure-servicebus.rules.nameProblem.charset")
                  : t("board.azure-servicebus.rules.nameHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel>{t("board.azure-servicebus.rules.kind")}</FieldLabel>
            <div style={{ display: "flex", gap: "6px" }}>
              <Button
                type="button"
                size="sm"
                variant={draft.kind === "sql" ? "default" : "outline"}
                onClick={() => set("kind", "sql")}
              >
                {t("board.azure-servicebus.rules.kindSql")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant={draft.kind === "correlation" ? "default" : "outline"}
                onClick={() => set("kind", "correlation")}
              >
                {t("board.azure-servicebus.rules.kindCorrelation")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant={draft.kind === "true" ? "default" : "outline"}
                onClick={() => set("kind", "true")}
              >
                {t("board.azure-servicebus.rules.kindTrue")}
              </Button>
            </div>
            <FieldDescription>{t("board.azure-servicebus.rules.kindHint")}</FieldDescription>
          </Field>

          {draft.kind === "sql" && (
            <Field>
              <FieldLabel htmlFor="sb-rule-expression">
                {t("board.azure-servicebus.rules.expression")}
              </FieldLabel>
              <Input
                id="sb-rule-expression"
                className="mono3"
                value={draft.expression}
                placeholder="colour = 'red'"
                onChange={(event) => set("expression", event.target.value)}
              />
              <FieldDescription>
                {t("board.azure-servicebus.rules.expressionHint")}
              </FieldDescription>
            </Field>
          )}

          {draft.kind === "correlation" && (
            <Field>
              <FieldLabel htmlFor="sb-rule-subject">
                {t("board.azure-servicebus.rules.subject")}
              </FieldLabel>
              <Input
                id="sb-rule-subject"
                className="mono3"
                value={draft.subject}
                placeholder="order"
                onChange={(event) => set("subject", event.target.value)}
              />
              <FieldDescription>
                {t("board.azure-servicebus.rules.subjectHint")}
              </FieldDescription>
            </Field>
          )}

          <Field>
            <FieldLabel htmlFor="sb-rule-action">
              {t("board.azure-servicebus.rules.action")}
            </FieldLabel>
            <Input
              id="sb-rule-action"
              className="mono3"
              value={draft.action}
              placeholder="SET routed = 'yes'"
              onChange={(event) => set("action", event.target.value)}
            />
            <FieldDescription>{t("board.azure-servicebus.rules.actionHint")}</FieldDescription>
          </Field>

          {error != null && (
            <FieldDescription style={{ color: "var(--c-danger)" }}>{error}</FieldDescription>
          )}
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={
              topic == null ||
              subscription == null ||
              name == null ||
              needsExpression ||
              needsSubject ||
              busy
            }
            onClick={() => void submit()}
          >
            {busy && <Spinner />}
            {t("common.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
