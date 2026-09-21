import { Section } from "@/components/site/ui";

const FACTS: [string, string][] = [
  [
    "Sessions outlive the window",
    "The shells run in a background daemon. Close the terminal, lose the SSH connection, come back with slat — the output is still there.",
  ],
  [
    "Panes, tabs and workspaces",
    "Split a pane either way, move between them by direction, walk a pane around the layout, zoom one to full size, group tabs into named workspaces.",
  ],
  [
    "Scroll back and search",
    "Every pane keeps its own history, with vim keys and smart-case search. A pane's screen survives splitting, closing and switching tabs.",
  ],
  [
    "Only what changed is drawn",
    "slat paints from its own terminal emulator and sends the difference, so half a million lines of output arrive in about a third of a second.",
  ],
  [
    "One binary, no runtime",
    "Static Go, nothing to install alongside it. The config file is optional — and a mistake in it is reported when you start, never ignored.",
  ],
  [
    "Windows, properly",
    "Panes run on a ConPTY and the socket is an AF_UNIX socket, so the same session model works there. The test suite runs on all three platforms.",
  ],
];

export function What() {
  return (
    <Section
      marker="slat"
      title="One window, however many shells"
      lead="It does the multiplexer things, and gets out of the way. No plugin manager, no server config, no second language to learn."
    >
      <dl className="grid gap-x-14 gap-y-0 sm:grid-cols-2">
        {FACTS.map(([title, body]) => (
          <div key={title} className="hairline-top py-5">
            <dt className="text-[16px] font-medium text-ink">{title}</dt>
            <dd className="mt-1.5 max-w-[46ch] text-[15px] leading-[1.6] text-ink-muted">
              {body}
            </dd>
          </div>
        ))}
      </dl>
    </Section>
  );
}
