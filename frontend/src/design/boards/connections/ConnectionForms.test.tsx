import { beforeAll, describe, expect, it, vi } from "vitest";

/**
 * The RocketMQ connection form, rendered.
 *
 * connectionDraft.test.ts covers the translation between the draft and what
 * ConnectionService stores, which is where the credentials rules live. What it
 * cannot see is whether a field is drawn at all: the advanced block held a
 * disabled placeholder for the namespace for as long as the driver had nowhere
 * to put one, and a disabled control reads exactly like a live one to every
 * test that only looks at the draft.
 *
 * Nothing here initialises i18n, so labels come back as their keys. That is
 * what is asserted on - whether the bundle resolves them is i18nCoverage's job.
 *
 * The import is dynamic because the component barrel reaches the Wails runtime
 * at module load, which wants a `window` this environment has to install first.
 */

const KEY = "page.connections.form.rocketmq";

let renderRocketMQForm: (namespace: string) => string;

beforeAll(async () => {
  const storage = { getItem: () => null, setItem() {}, removeItem() {} };
  vi.stubGlobal("window", {
    _wails: { environment: { OS: "darwin" } },
    matchMedia: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }),
    localStorage: storage,
    addEventListener() {},
    removeEventListener() {},
  });
  vi.stubGlobal("localStorage", storage);

  const [{ renderToStaticMarkup }, forms] = await Promise.all([
    import("react-dom/server"),
    import("./ConnectionForms"),
  ]);

  renderRocketMQForm = (namespace: string) =>
    renderToStaticMarkup(
      <forms.RocketMQForm
        value={{ ...forms.emptyRocketMQDraft(), namespace }}
        onChange={() => {}}
      />,
    );
});

/**
 * The markup of one field, from its label to the end of that field's block.
 *
 * Split on the closing tag as well: the label key is a prefix of its own hint
 * key, so the bare key matches twice and the first half of the field is lost.
 */
function fieldAfter(html: string, labelKey: string): string {
  const rest = html.split(`${labelKey}</span>`)[1] ?? "";
  return rest.slice(0, rest.indexOf("</div>"));
}

describe("the RocketMQ connection form", () => {
  it("draws the namespace as a live field, not the placeholder it replaced", () => {
    const html = renderRocketMQForm("MQ_INST_1");

    const namespace = fieldAfter(html, `${KEY}.namespace`);
    expect(namespace).toContain('value="MQ_INST_1"');
    // The attribute, not the substring: Tailwind's disabled: variants put the
    // word in every input's class list.
    expect(namespace).not.toContain('disabled=""');

    // The control this replaced, and proof the check above can tell the
    // difference: the two fields beside it are still placeholders.
    expect(html).not.toContain("instanceId");
    expect(fieldAfter(html, `${KEY}.traceTopic`)).toContain('disabled=""');
  });

  it("puts a field's explanation under its control, not above it", () => {
    // A hint on the label line ran the two together, and made the row as tall
    // as the longest explanation in it - which left the short field beside it
    // an empty band where its own label belonged.
    const html = renderRocketMQForm("MQ_INST_1");

    const label = html.indexOf(`${KEY}.namespace</span>`);
    const input = html.indexOf('value="MQ_INST_1"');
    const hint = html.indexOf(`${KEY}.namespaceHint`);

    expect(label).toBeGreaterThan(-1);
    expect(label).toBeLessThan(input);
    expect(input).toBeLessThan(hint);
  });

  it("opens the advanced block by itself when a namespace is set", () => {
    // Otherwise editing a scoped connection would hide the very thing that
    // decides which topics it can see.
    expect(renderRocketMQForm("MQ_INST_1")).toContain(`${KEY}.namespace`);
    expect(renderRocketMQForm("")).not.toContain(`${KEY}.namespace`);
  });
});
