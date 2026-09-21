import { Agents } from "@/components/site/agents";
import { Footer } from "@/components/site/footer";
import { Hero } from "@/components/site/hero";
import { Install } from "@/components/site/install";
import { Keys } from "@/components/site/keys";
import { Look } from "@/components/site/look";
import { Nav } from "@/components/site/nav";
import { Watch } from "@/components/site/watch";
import { What } from "@/components/site/what";

export default function Page() {
  return (
    <>
      {/* The blueprint grid, and nothing else, behind the whole page. */}
      <div className="grid-field pointer-events-none fixed inset-0 -z-10" aria-hidden />
      <Nav />
      <main>
        <Hero />
        <What />
        <Watch />
        <Agents />
        <Look />
        <Install />
        <Keys />
      </main>
      <Footer />
    </>
  );
}
