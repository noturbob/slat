import { Section } from "@/components/site/ui";

const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";

/**
 * The simulated session above is faithful, but it is still a simulation.
 * This is the program itself, recorded.
 */
export function Watch() {
  return (
    <Section
      marker="slat   # in a real terminal"
      title="Here it is, actually running"
      lead="Splitting, running two things at once, scrolling back through the output, then detaching and picking the session up again with everything still alive."
    >
      <figure>
        <div className="glass overflow-hidden rounded-2xl p-2">
          {/* The same film the README carries, as video rather than the GIF:
              a quarter of the bytes, and it does not re-download on loop. */}
          <video
            className="w-full rounded-xl"
            src={`${base}/demo.mp4`}
            poster={`${base}/demo-poster.jpg`}
            controls
            playsInline
            preload="none"
            aria-label="A recording of slat: splitting panes, running two commands, searching and copying a line, detaching, listing the session from outside, and reattaching"
          />
        </div>
      </figure>
    </Section>
  );
}
