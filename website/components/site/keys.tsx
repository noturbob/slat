import { Section } from "@/components/site/ui";

/** The defaults, as internal/config's defaultKeybinds lists them. */
const GROUPS: { title: string; keys: [string, string][] }[] = [
  {
    title: "Panes",
    keys: [
      ["v  h", "split left / right, top / bottom"],
      ["k j H L", "focus the pane above, below, left, right (or the arrows)"],
      ["m", "move mode: walk the pane around with hjkl, q leaves"],
      ["< > K J", "move the pane left, right, up, down"],
      ["s", "swap with the next pane"],
      ["+ - =", "grow, shrink, equalize"],
      ["z", "zoom the pane to full size"],
      ["x", "close the pane"],
      ["[", "scroll back, search, select and copy (v, V, y)"],
    ],
  },
  {
    title: "Tabs, workspaces, session",
    keys: [
      ["c  n p", "new tab, next, previous"],
      [",  X", "rename the tab, close it"],
      ["W  w P", "new workspace, next, previous"],
      ["$", "rename the workspace"],
      ["d", "detach — everything keeps running"],
      ["q", "quit"],
      ["?", "show every binding"],
    ],
  },
];

export function Keys() {
  return (
    <Section
      id="keys"
      fill
      marker="slat --help"
      title="Everything is a keystroke away"
      lead="Press the prefix — Ctrl-S by default — then one key. Press it twice to send it through to the program. Every binding below can be changed, and two commands can never end up fighting over the same key."
    >
      <div className="grid gap-x-14 gap-y-10 lg:grid-cols-2">
        {GROUPS.map((group) => (
          <div key={group.title}>
            <h3 className="font-mono text-[15px] text-ink">{group.title}</h3>
            <ul className="mt-3">
              {group.keys.map(([keys, what]) => (
                <li
                  key={keys}
                  className="hairline-top grid gap-0.5 py-3 sm:grid-cols-[minmax(0,12ch)_minmax(0,1fr)] sm:items-baseline sm:gap-6"
                >
                  <code className="font-mono text-[13.5px] whitespace-nowrap text-ink">
                    <span className="text-ink-faint">C-s</span> {keys}
                  </code>
                  <span className="text-[14.5px] text-ink-muted">{what}</span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </Section>
  );
}
