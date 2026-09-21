import type { Metadata } from "next";
import { IBM_Plex_Mono, IBM_Plex_Sans } from "next/font/google";

import "./globals.css";

const plexSans = IBM_Plex_Sans({
  variable: "--font-plex-sans",
  subsets: ["latin"],
  weight: ["400", "500", "600"],
});

const plexMono = IBM_Plex_Mono({
  variable: "--font-plex-mono",
  subsets: ["latin"],
  weight: ["400", "500", "600"],
});

const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";

export const metadata: Metadata = {
  metadataBase: new URL("https://noturbob.github.io/slat/"),
  title: "slat — a terminal multiplexer",
  description:
    "Split your terminal into panes, tabs and workspaces. Sessions keep running when you close the window, and scripts or AI agents can drive them from outside. Linux, macOS and Windows.",
  keywords: ["terminal multiplexer", "tmux alternative", "slat", "Go", "ConPTY", "AI agents"],
  openGraph: {
    title: "slat — a terminal multiplexer",
    description:
      "Panes, tabs and workspaces that outlive the window — and a command line agents can drive.",
    url: "https://noturbob.github.io/slat/",
    siteName: "slat",
    images: [{ url: "social-preview.png", width: 1280, height: 640 }],
    type: "website",
  },
  twitter: { card: "summary_large_image" },
  icons: { icon: `${base}/icon.svg` },
};

/**
 * The theme is settled before the first paint: without this the page shows
 * one frame of the wrong one, which is exactly the flicker the animated
 * toggle exists to avoid.
 */
const themeScript = `
(function () {
  try {
    var saved = localStorage.getItem("theme");
    var dark = saved ? saved === "dark" : true;
    document.documentElement.classList.toggle("dark", dark);
  } catch (e) {
    document.documentElement.classList.add("dark");
  }
})();
`;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={`${plexSans.variable} ${plexMono.variable}`}
    >
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body className="antialiased">{children}</body>
    </html>
  );
}
