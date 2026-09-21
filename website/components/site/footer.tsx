import { Heart } from "lucide-react";

import { GitHubMark } from "@/components/site/icons";

import { Logo } from "@/components/site/logo";

const LINKS: [string, string][] = [
  ["Source", "https://github.com/noturbob/slat"],
  ["Releases", "https://github.com/noturbob/slat/releases"],
  ["Man page", "https://github.com/noturbob/slat/blob/main/slat.1"],
  ["Agent CLI design", "https://github.com/noturbob/slat/blob/main/docs/design/agent-cli.md"],
  ["Rices", "https://github.com/noturbob/slat/tree/main/docs/rices"],
];

export function Footer() {
  return (
    <footer className="hairline-top mt-10">
      <div className="mx-auto flex w-full max-w-[1120px] flex-col gap-8 px-5 py-14 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2.5 text-ink">
            <Logo size={18} />
            <span className="font-mono text-[16px] font-medium">slat</span>
          </div>
          <p className="mt-3 max-w-[42ch] text-[14px] leading-[1.6] text-ink-muted">
            A terminal multiplexer, one year old this October. MIT licensed, and small enough
            to read in an evening.
          </p>
          <a
            href="https://ko-fi.com/bobbyanthene"
            className="mt-4 inline-flex items-center gap-2 text-[14px] text-ink-muted transition-colors hover:text-ink"
          >
            <Heart size={14} className="text-cyan" />
            Support the work
          </a>
        </div>

        <nav className="flex flex-col gap-2.5 text-[14px]">
          {LINKS.map(([label, href]) => (
            <a key={href} href={href} className="text-ink-muted transition-colors hover:text-ink">
              {label}
            </a>
          ))}
          <a
            href="https://github.com/noturbob/slat"
            className="mt-1 inline-flex items-center gap-2 text-ink-muted transition-colors hover:text-ink"
          >
            <GitHubMark size={14} />
            noturbob/slat
          </a>
        </nav>
      </div>
    </footer>
  );
}
