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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { formatErrorMessage } from "@/lib/utils";

/**
 * The name a Classic subscription needs, or null when it is not yet whole.
 *
 * Classic identifies a durable subscription by the pair (client id,
 * subscription name), and the canonical ref carries one string - so the two
 * are joined by a vertical bar, chosen because JMS client ids routinely
 * contain a slash or a colon and a separator that appears inside a name cannot
 * be split back out.
 *
 * Exported because it is the rule this form exists to enforce: a Classic
 * subscription submitted with half a name is accepted by nothing and the error
 * comes back from the driver rather than from the field.
 */
export function classicSubscriptionRef(clientId: string, name: string): string | null {
  const client = clientId.trim();
  const subscription = name.trim();
  if (client === "" || subscription === "") return null;
  if (client.includes("|")) return null;
  return `${client}|${subscription}`;
}

/**
 * A topic, a name, and on Classic a client id.
 *
 * The topic is chosen rather than typed: only a topic can hold a durable
 * subscription - a JMS queue's consumers are connections with no name and no
 * stored position - and offering a free-text field would let somebody name a
 * queue and get an error the form could have prevented.
 *
 * The client id appears only for Classic, because only Classic has one.
 * Artemis identifies a subscription by the name of the queue bound to the
 * address, and there is nothing else to ask for.
 */
export function SubscriptionDialogActiveMQ({
  open,
  onOpenChange,
  topics,
  product,
  onCreate,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  topics: readonly string[];
  product: "classic" | "artemis";
  onCreate: (topic: string, name: string, selector: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [topic, setTopic] = useState("");
  const [name, setName] = useState("");
  const [clientId, setClientId] = useState("");
  const [selector, setSelector] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setTopic(topics[0] ?? "");
      setName("");
      setClientId("");
      setSelector("");
      setError(null);
    }
  }, [open, topics]);

  const ref =
    product === "classic" ? classicSubscriptionRef(clientId, name) : name.trim() || null;
  const ready = topic !== "" && ref != null;

  const submit = async () => {
    if (!ready || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onCreate(topic, ref, selector.trim());
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
          <DialogTitle>{t("board.activemq.subscriptions.createTitle")}</DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel>{t("board.activemq.subscriptions.topic")}</FieldLabel>
            <Select value={topic} onValueChange={setTopic}>
              <SelectTrigger>
                <SelectValue placeholder={t("board.activemq.subscriptions.topicPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                {topics.map((option) => (
                  <SelectItem key={option} value={option}>
                    {option}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldDescription>
              {t("board.activemq.subscriptions.topicHint")}
            </FieldDescription>
          </Field>
          {product === "classic" && (
            <Field>
              <FieldLabel>{t("board.activemq.subscriptions.clientId")}</FieldLabel>
              <Input
                className="mono3"
                value={clientId}
                onChange={(event) => setClientId(event.target.value)}
              />
              <FieldDescription>
                {t("board.activemq.subscriptions.clientIdHint")}
              </FieldDescription>
            </Field>
          )}
          <Field>
            <FieldLabel>{t("board.activemq.subscriptions.name")}</FieldLabel>
            <Input
              className="mono3"
              value={name}
              onChange={(event) => setName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") void submit();
              }}
            />
          </Field>
          <Field>
            <FieldLabel>{t("board.activemq.subscriptions.selector")}</FieldLabel>
            <Input
              className="mono3"
              value={selector}
              placeholder="JMSPriority > 4"
              onChange={(event) => setSelector(event.target.value)}
            />
            <FieldDescription>
              {t("board.activemq.subscriptions.selectorHint")}
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
          <Button disabled={!ready || busy} onClick={() => void submit()}>
            {busy && <Spinner />}
            {t("common.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
