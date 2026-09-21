"use client";

import { GitHubMark } from "@/components/site/icons";

import { AnimatedThemeToggler } from "@/components/ui/animated-theme-toggler";
import { Logo } from "@/components/site/logo";

const LINKS = [
  { href: "#agents", label: "Agents" },
  { href: "#look", label: "Make it yours" },
  { href: "#install", label: "Install" },
  { href: "#keys", label: "Keys" },
];

export function Nav() {
  return (
    <header className="fixed inset-x-0 top-0 z-50">
      <nav className="mx-auto flex h-16 w-full max-w-[1120px] items-center gap-6 px-5">
        <a href="#top" className="flex items-center gap-2.5 text-ink" aria-label="slat, home">
          <Logo size={26} />
          <span className="font-mono text-[19px] font-medium tracking-tight">slat</span>
        </a>

        <div className="hidden items-center gap-1 md:flex">
          {LINKS.map((link) => (
            <a
              key={link.href}
              href={link.href}
              className="rounded-full px-3 py-1.5 text-[14px] text-ink-muted transition-colors hover:text-ink"
            >
              {link.label}
            </a>
          ))}
        </div>

        <div className="ml-auto flex items-center gap-1.5">
          <AnimatedThemeToggler
            variant="circle"
            duration={1800}
            className="liquid inline-flex size-9 items-center justify-center rounded-full text-ink hover:text-ink [&_svg]:size-4"
          />
          <a
            href="https://github.com/noturbob/slat"
            className="liquid inline-flex items-center gap-2 rounded-full px-3.5 py-2 text-[14px] text-ink"
          >
            <GitHubMark size={15} />
            <span className="hidden sm:inline">GitHub</span>
          </a>
        </div>
      </nav>
    </header>
  );
}
