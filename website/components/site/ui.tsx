"use client";

import { useState } from "react";
import { Check, Copy } from "lucide-react";

import { cn } from "@/lib/utils";

/**
 * A section opens with the command that does the thing it describes, set in
 * mono at body size. It marks the section the way an eyebrow label would,
 * but it carries something you can actually run.
 */
export function Marker({ children }: { children: string }) {
  return (
    <p className="font-mono text-[13px] text-ink-faint">
      <span className="select-none text-cyan">$ </span>
      {children}
    </p>
  );
}

export function Section({
  id,
  marker,
  title,
  lead,
  children,
  className,
}: {
  id?: string;
  marker: string;
  title: string;
  lead?: string;
  children?: React.ReactNode;
  className?: string;
}) {
  return (
    <section id={id} className={cn("mx-auto w-full max-w-[1120px] px-5 py-20 sm:py-28", className)}>
      <div className="max-w-[62ch]">
        <Marker>{marker}</Marker>
        <h2 className="mt-4 font-mono text-[26px] leading-[1.15] font-medium tracking-tight text-ink sm:text-[32px]">
          {title}
        </h2>
        {lead ? <p className="mt-4 text-[17px] leading-[1.6] text-ink-muted">{lead}</p> : null}
      </div>
      {children}
    </section>
  );
}

export function Pill({
  href,
  onClick,
  children,
  variant = "ghost",
  className,
  ...props
}: {
  href?: string;
  onClick?: () => void;
  children: React.ReactNode;
  variant?: "ghost" | "solid";
  className?: string;
} & React.ComponentPropsWithoutRef<"a">) {
  const base =
    "inline-flex items-center gap-2 rounded-full px-4 py-2 text-[14px] font-medium transition-colors";
  const look =
    variant === "solid"
      ? "bg-cyan text-canvas hover:brightness-110"
      : "text-ink hover:bg-glass-strong";
  const edge = variant === "solid" ? undefined : { boxShadow: "inset 0 0 0 1px var(--hairline)" };

  if (href) {
    return (
      <a href={href} className={cn(base, look, className)} style={edge} {...props}>
        {children}
      </a>
    );
  }
  return (
    <button type="button" onClick={onClick} className={cn(base, look, className)} style={edge}>
      {children}
    </button>
  );
}

/** A command you can copy. The copy button says what happened, once. */
export function CommandLine({ command, comment }: { command: string; comment?: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopied(false);
    }
  };
  return (
    <div className="group flex items-start gap-3 py-2.5">
      <code className="min-w-0 flex-1 font-mono text-[13.5px] leading-[1.7] break-words text-ink">
        <span className="select-none text-ink-faint">$ </span>
        {command}
        {comment ? <span className="text-ink-faint">  # {comment}</span> : null}
      </code>
      <button
        type="button"
        onClick={copy}
        aria-label={copied ? "Copied" : `Copy: ${command}`}
        className="rounded-md p-1.5 text-ink-faint transition-colors hover:text-ink"
      >
        {copied ? <Check size={14} className="text-cyan" /> : <Copy size={14} />}
      </button>
    </div>
  );
}

/** A choice among a few: the theme, the border style, the platform. */
export function Choice({
  options,
  value,
  onChange,
  label,
}: {
  options: readonly string[];
  value: string;
  onChange: (value: string) => void;
  label: string;
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label={label}>
      {options.map((option) => {
        const selected = option === value;
        return (
          <button
            key={option}
            type="button"
            onClick={() => onChange(option)}
            aria-pressed={selected}
            className={cn(
              "rounded-full px-3 py-1.5 font-mono text-[13px] transition-colors",
              selected ? "text-ink" : "text-ink-faint hover:text-ink-muted"
            )}
            style={selected ? { boxShadow: "inset 0 0 0 1px var(--hairline)" } : undefined}
          >
            {option}
          </button>
        );
      })}
    </div>
  );
}
