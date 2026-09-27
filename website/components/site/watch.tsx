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
      lead="One terminal split any way you like, panes walked around the layout, tabs and workspaces named in place, the scrollback searched and copied over ssh, a session that survives a reboot, and the whole thing driven from outside."
    >
      <figure>
        <div className="glass overflow-hidden rounded-2xl p-2">
          {/* The whole film. The README carries an excerpt of it as a GIF,
              because GitHub will not play video in a readme; here there is
              no such excuse, so the video is the full forty seconds. */}
          <video
            className="w-full rounded-xl"
            src={`${base}/demo.mp4`}
            poster={`${base}/demo-poster.jpg`}
            controls
            playsInline
            preload="none"
            aria-label="A recording of slat: splitting panes, walking one around the layout, naming tabs and workspaces, searching and copying from the scrollback, detaching and reattaching, being driven by a script, and changing palette"
          />
        </div>
      </figure>
    </Section>
  );
}
