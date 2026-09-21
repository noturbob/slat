import type { NextConfig } from "next";

/**
 * The site is published by GitHub Pages from the slat repository, at
 * /slat — so this exports plain files and prefixes every asset path. Set
 * SLAT_BASE_PATH="" to build it for a root domain instead.
 */
const basePath = process.env.SLAT_BASE_PATH ?? "/slat";

const nextConfig: NextConfig = {
  output: "export",
  basePath,
  assetPrefix: basePath || undefined,
  trailingSlash: true,
  images: { unoptimized: true },
  env: { NEXT_PUBLIC_BASE_PATH: basePath },
};

export default nextConfig;
