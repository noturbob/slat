"use client";

import { useEffect, useState } from "react";
import { GitHubMark } from "@/components/site/icons";

import { AnimatedThemeToggler } from "@/components/ui/animated-theme-toggler";
import { Logo } from "@/components/site/logo";
import { cn } from "@/lib/utils";

const LINKS = [
  { href: "#agents", label: "Agents" },
  { href: "#look", label: "Make it yours" },
  { href: "#install", label: "Install" },
  { href: "#keys", label: "Keys" },
];

export function Nav() {
  const [lifted, setLifted] = useState(false);

  useEffect(() => {
    const onScroll = () => setLifted(window.scrollY > 12);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  return (
    <header
      className={cn(
        "fixed inset-x-0 top-0 z-50 transition-colors duration-300",
        lifted && "bg-canvas/80 backdrop-blur-xl"
      )}
      style={lifted ? { boxShadow: "inset 0 -1px 0 0 var(--hairline)" } : undefined}
    >
      <nav className="mx-auto flex h-16 w-full max-w-[1120px] items-center gap-6 px-5">
        <a href="#top" className="flex items-center gap-2.5 text-ink" aria-label="slat, home">
          <Logo size={18} />
          <span className="font-mono text-[17px] font-medium tracking-tight">slat</span>
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
            className="inline-flex size-9 items-center justify-center rounded-full text-ink-muted transition-colors hover:text-ink [&_svg]:size-4"
            style={{ boxShadow: "inset 0 0 0 1px var(--hairline)" }}
          />
          <a
            href="https://github.com/noturbob/slat"
            className="inline-flex items-center gap-2 rounded-full px-3.5 py-2 text-[14px] text-ink transition-colors hover:bg-glass-strong"
            style={{ boxShadow: "inset 0 0 0 1px var(--hairline)" }}
          >
            <GitHubMark size={15} />
            <span className="hidden sm:inline">GitHub</span>
          </a>
        </div>
      </nav>
    </header>
  );
}
