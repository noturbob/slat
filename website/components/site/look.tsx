"use client";

import { useState } from "react";

import { Session } from "@/components/site/session";
import { Choice, Section } from "@/components/site/ui";
import { BORDER_STYLES, THEMES, type BorderStyle } from "@/lib/themes";

export function Look() {
  const [themeName, setThemeName] = useState(THEMES[0].name);
  const [border, setBorder] = useState<BorderStyle>("sharp");
  const theme = THEMES.find((t) => t.name === themeName) ?? THEMES[0];

  const config = [
    `[theme]`,
    `name = "${theme.name}"`,
    ``,
    `[borders]`,
    `style = "${border}"`,
    ``,
    `[status]`,
    `left  = " {workspace} │ {tabs}"`,
    `right = "{badge} pane {pane}/{panes} "`,
    `tab   = " {index}:{name}{alert} "`,
    ``,
    `[animation]`,
    `split  = "90ms"`,
    `move   = "120ms"`,
    `easing = "out-cubic"`,
  ].join("\n");

  return (
    <Section
      id="look"
      marker="$EDITOR ~/.config/slat/config.toml"
      title="Make it yours"
      lead="Colour, shape and motion all belong to whoever runs it: five palettes or your own, six border styles or your own glyphs, a status bar written as a format string, and animation timing per event. Pick two and watch the session change."
    >
      <div className="mt-10 grid gap-8 lg:grid-cols-[minmax(0,1.15fr)_minmax(0,0.85fr)] lg:gap-12">
        <div>
          <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
            <div>
              <p className="mb-1.5 text-[13px] text-ink-faint">theme</p>
              <Choice
                label="Theme"
                options={THEMES.map((t) => t.name)}
                value={themeName}
                onChange={setThemeName}
              />
            </div>
            <div>
              <p className="mb-1.5 text-[13px] text-ink-faint">borders</p>
              <Choice
                label="Border style"
                options={BORDER_STYLES}
                value={border}
                onChange={(value) => setBorder(value as BorderStyle)}
              />
            </div>
          </div>

          <div className="glass mt-6 rounded-2xl p-3 sm:p-4">
            <Session theme={theme} borderStyle={border} start={["shell", "logs"]} />
          </div>
        </div>

        <div>
          <pre className="glass overflow-x-auto rounded-2xl p-5 font-mono text-[13px] leading-[1.7] text-ink-muted">
            <code>{config}</code>
          </pre>
          <div className="mt-6 space-y-4 text-[15px] leading-[1.6] text-ink-muted">
            <p>
              A theme colours what slat draws — the bar, the borders, its overlays. Programs in
              your panes keep their own colours, and fonts belong to your terminal, not to slat.
            </p>
            <p>
              Every key is rebindable, and a mistake anywhere — an unknown colour, a glyph two
              columns wide, a placeholder that doesn&apos;t exist — is refused when slat starts
              instead of drawn badly later.
            </p>
            <p className="text-ink-faint">
              Four finished setups live in{" "}
              <a
                className="text-ink underline decoration-hairline underline-offset-4 hover:decoration-current"
                href="https://github.com/noturbob/slat/tree/main/docs/rices"
              >
                docs/rices
              </a>
              , including one that is ASCII and sixteen colours for a bare console. Send yours.
            </p>
          </div>
        </div>
      </div>
    </Section>
  );
}
