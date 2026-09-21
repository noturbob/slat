"use client";

import { useEffect, useRef, useState } from "react";
import { ArrowDown } from "lucide-react";

import { GitHubMark } from "@/components/site/icons";

import { Session } from "@/components/site/session";
import { Pill } from "@/components/site/ui";
import { THEMES } from "@/lib/themes";

/**
 * Rows the hero's session can afford: the screen, less what is actually
 * above and below it. The headline is measured rather than guessed at,
 * because it wraps differently at every width — a short laptop gets a
 * shorter terminal instead of a caption pushed off the bottom.
 */
function useFittedRows(copy: React.RefObject<HTMLDivElement | null>, caption: React.RefObject<HTMLParagraphElement | null>) {
  const [rows, setRows] = useState(16);

  useEffect(() => {
    const fit = () => {
      const copyHeight = copy.current?.getBoundingClientRect().height ?? 380;
      const captionHeight = caption.current?.getBoundingClientRect().height ?? 22;
      const cellSize = parseFloat(
        getComputedStyle(document.documentElement).getPropertyValue("--cell-size")
      );
      const cell = (cellSize || 13) * 1.35;
      // Padding above and below the section, the gap between the two
      // blocks, the glass around the terminal, and the row of keys.
      const chrome = 316; // measured: padding, gap, glass, keys row, caption margin
      const room = window.innerHeight - copyHeight - captionHeight - chrome;
      setRows(Math.max(8, Math.min(22, Math.floor(room / cell))));
    };
    fit();
    const observer = new ResizeObserver(fit);
    if (copy.current) observer.observe(copy.current);
    window.addEventListener("resize", fit);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", fit);
    };
  }, [caption, copy]);

  return rows;
}

export function Hero() {
  const copyRef = useRef<HTMLDivElement>(null);
  const captionRef = useRef<HTMLParagraphElement>(null);
  const rows = useFittedRows(copyRef, captionRef);
  return (
    <section
      id="top"
      className="relative flex min-h-svh flex-col justify-center gap-[var(--block-gap)] pt-20 pb-16"
    >
      <div ref={copyRef} className="mx-auto w-full max-w-[1120px] px-5">
        <p className="font-mono text-[13px] text-ink-faint">
          v1.0 · Linux, macOS, Windows · MIT
        </p>

        <h1 className="mt-5 max-w-[20ch] font-mono text-[34px] leading-[1.08] font-medium tracking-tight text-ink sm:text-[54px]">
          Split the terminal.
          <br />
          Keep it running.
          <span className="ml-1 inline-block h-[0.78em] w-[0.5em] translate-y-[0.04em] bg-cyan align-baseline" />
        </h1>

        <p className="hero-lead mt-6 max-w-[64ch] text-[17px] leading-[1.65] text-ink-muted sm:text-[19px]">
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

      <div className="session-halo relative mx-auto w-full max-w-[1120px] px-5">
        <div className="glass min-w-0 rounded-2xl p-3 sm:p-4">
          <Session theme={THEMES[0]} rows={rows} start={["shell", "build"]} />
        </div>
        <p ref={captionRef} className="mt-4 text-[14px] leading-[1.6] text-ink-faint">
          Not a screenshot: slat&apos;s own layout engine, ported to the page. Press the keys.
        </p>
      </div>
    </section>
  );
}
