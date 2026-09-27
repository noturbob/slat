"use client";

import { useEffect, useRef } from "react";

import { Section } from "@/components/site/ui";

const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";

/**
 * The simulated session above is faithful, but it is still a simulation.
 * This is the program itself, recorded.
 *
 * It starts when it comes into view and stops when it leaves, so a visitor
 * who never scrolls this far never pays for the download, and a tab left
 * open somewhere else is not quietly decoding video.
 */
export function Watch() {
  const video = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    const el = video.current;
    if (!el) return;

    // React sets muted as an attribute, which browsers do not always honour
    // in time for the first play(); the property always counts.
    el.muted = true;

    // Anyone who has asked for less motion gets the poster and the controls,
    // and decides for themselves.
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          // Autoplay can still be refused — a data saver, a browser policy,
          // a user setting. The controls are there when it is.
          void el.play().catch(() => {});
        } else if (!el.paused) {
          el.pause();
        }
      },
      // Enough of it on screen to be worth watching, rather than a sliver
      // at the edge of the viewport.
      { threshold: 0.35 },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

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
            ref={video}
            className="w-full rounded-xl"
            src={`${base}/demo.mp4`}
            poster={`${base}/demo-poster.jpg`}
            controls
            loop
            muted
            playsInline
            preload="none"
            aria-label="A recording of slat: splitting panes, walking one around the layout, naming tabs and workspaces, searching and copying from the scrollback, detaching and reattaching, being driven by a script, and changing palette"
          />
        </div>
      </figure>
    </Section>
  );
}
