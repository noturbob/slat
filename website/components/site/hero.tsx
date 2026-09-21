"use client";

import { ArrowDown } from "lucide-react";

import { GitHubMark } from "@/components/site/icons";

import { Session } from "@/components/site/session";
import { Pill } from "@/components/site/ui";
import { THEMES } from "@/lib/themes";

export function Hero() {
  return (
    <section id="top" className="relative pt-28 pb-8 sm:pt-36">
      <div className="mx-auto w-full max-w-[1120px] px-5">
        <p className="font-mono text-[13px] text-ink-faint">
          v1.0 · Linux, macOS, Windows · MIT
        </p>

        <h1 className="mt-5 max-w-[20ch] font-mono text-[34px] leading-[1.08] font-medium tracking-tight text-ink sm:text-[54px]">
          Split the terminal.
          <br />
          Keep it running.
          <span className="ml-1 inline-block h-[0.78em] w-[0.5em] translate-y-[0.04em] bg-cyan align-baseline" />
        </h1>

        <p className="mt-6 max-w-[64ch] text-[17px] leading-[1.65] text-ink-muted sm:text-[19px]">
          A terminal multiplexer written in Go: tiling panes, tabs and workspaces in one
          window, and shells that keep running after you close it. New in 1.0 — every pane can
          be driven from outside, so a script or a coding agent can start work, wait for it,
          and read what it printed.
        </p>

        <div className="mt-8 flex flex-wrap items-center gap-2.5">
          <Pill href="#install" variant="solid">
            Install slat
            <ArrowDown size={15} />
          </Pill>
          <Pill href="https://github.com/noturbob/slat">
            <GitHubMark size={15} />
            Read the source
          </Pill>
        </div>
      </div>

      <div className="session-halo relative mx-auto mt-14 w-full max-w-[1120px] px-5">
        <div className="glass rounded-2xl p-3 sm:p-4">
          <Session theme={THEMES[0]} start={["shell", "build"]} />
        </div>
        <p className="mt-4 max-w-[70ch] text-[14px] leading-[1.6] text-ink-faint">
          Not a screenshot: slat&apos;s layout engine, border glyphs and status bar, ported to
          the page. The keys under it are the real ones — press them.
        </p>
      </div>
    </section>
  );
}
