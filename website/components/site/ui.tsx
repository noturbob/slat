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
  fill = false,
  className,
}: {
  id?: string;
  marker: string;
  title: string;
  lead?: string;
  children?: React.ReactNode;
  /** Stand the section up to the height of the viewport and centre it, so
   *  scrolling to it lands on a whole screen rather than a fragment. */
  fill?: boolean;
  className?: string;
}) {
  return (
    <section
      id={id}
      className={cn(
        "mx-auto w-full max-w-[1120px] px-5 py-[var(--section-gap)]",
        fill && "flex min-h-svh flex-col justify-center",
        className
      )}
    >
      <div className="max-w-[62ch]">
        <Marker>{marker}</Marker>
        <h2 className="mt-4 font-mono text-[26px] leading-[1.15] font-medium tracking-tight text-ink sm:text-[32px]">
          {title}
        </h2>
        {lead ? <p className="mt-4 text-[17px] leading-[1.6] text-ink-muted">{lead}</p> : null}
      </div>
      {children ? <div className="mt-[var(--block-gap)]">{children}</div> : null}
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
    variant === "solid" ? "bg-cyan text-canvas hover:brightness-110" : "liquid text-ink";

  if (href) {
    return (
      <a href={href} className={cn(base, look, className)} {...props}>
        {children}
      </a>
    );
  }
  return (
    <button type="button" onClick={onClick} className={cn(base, look, className)}>
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
              selected ? "liquid text-ink" : "text-ink-muted hover:text-ink"
            )}
          >
            {option}
          </button>
        );
      })}
    </div>
  );
}
