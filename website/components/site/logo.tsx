/**
 * The mark: four slats, the lower three broken at one column so the gap
 * reads as the split between two panes. One colour, no background — it
 * takes currentColor, so it works on either theme without a second file.
 */
export function Logo({ className, size = 20 }: { className?: string; size?: number }) {
  return (
    <svg
      viewBox="0 0 64 64"
      width={size}
      height={size}
      fill="currentColor"
      className={className}
      aria-hidden
    >
      <rect x="8" y="12" width="48" height="7" rx="2" />
      <rect x="8" y="26" width="26" height="7" rx="2" />
      <rect x="38" y="26" width="18" height="7" rx="2" />
      <rect x="8" y="36" width="20" height="7" rx="2" />
      <rect x="38" y="36" width="18" height="7" rx="2" />
      <rect x="8" y="46" width="26" height="7" rx="2" />
      <rect x="38" y="46" width="12" height="7" rx="2" />
    </svg>
  );
}
