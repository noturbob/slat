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
          {/* Recorded by docs/demo.tape, so it is regenerated rather than
              re-shot whenever the program's look changes. */}
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            className="w-full rounded-xl"
            src={`${base}/demo.gif`}
            loading="lazy"
            decoding="async"
            alt="A recording of slat: splitting panes, running two commands, scrolling back, detaching and reattaching"
          />
        </div>
      </figure>
    </Section>
  );
}
