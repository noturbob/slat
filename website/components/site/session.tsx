"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  blit,
  cellAt,
  curtain,
  drawBorders,
  layout,
  newGrid,
  PANE_CONTENT,
  panes,
  statusBar,
  type CellStyle,
  type Node,
  type Pane,
  type Rect,
} from "@/lib/session";
import { BORDER_SETS, THEMES, type BorderStyle, type SlatTheme } from "@/lib/themes";

type Anim =
  | { kind: "reveal"; paneId: number; sideways: boolean; start: number; ms: number }
  | {
      kind: "slide";
      a: { id: number; from: Rect; to: Rect };
      b: { id: number; from: Rect; to: Rect };
      start: number;
      ms: number;
    }
  | null;

const ROWS = 22;
const REVEAL_MS = 320;
const SLIDE_MS = 380;

function leaf(pane: Pane): Node {
  return { kind: "leaf", pane };
}

function makePane(id: number, name: string, status: Pane["status"] = "idle"): Pane {
  return { id, name, lines: PANE_CONTENT[name] ?? PANE_CONTENT.shell, status };
}

/** The session a section opens with. One name is one pane, as `slat`
 *  gives you; two are split left and right. */
function initialTree(names: string[]): Node {
  const [first, ...rest] = names;
  let node: Node = leaf(makePane(1, first ?? "shell"));
  rest.forEach((name, i) => {
    node = {
      kind: "split",
      dir: "v",
      ratio: 0.5,
      a: node,
      b: leaf(makePane(i + 2, name, name === "build" ? "working" : "idle")),
    };
  });
  return node;
}

function replaceLeaf(node: Node, id: number, make: (pane: Pane) => Node): Node {
  if (node.kind === "leaf") return node.pane.id === id ? make(node.pane) : node;
  return { ...node, a: replaceLeaf(node.a, id, make), b: replaceLeaf(node.b, id, make) };
}

function removeLeaf(node: Node, id: number): Node | null {
  if (node.kind === "leaf") return node.pane.id === id ? null : node;
  const a = removeLeaf(node.a, id);
  const b = removeLeaf(node.b, id);
  if (!a) return b;
  if (!b) return a;
  return { ...node, a, b };
}

/** Swaps two panes in the tree, the way MovePaneInDirection does. */
function swapPanes(node: Node, x: number, y: number): Node {
  const find = (n: Node, id: number): Pane | null => {
    if (n.kind === "leaf") return n.pane.id === id ? n.pane : null;
    return find(n.a, id) ?? find(n.b, id);
  };
  const px = find(node, x);
  const py = find(node, y);
  if (!px || !py) return node;
  const walk = (n: Node): Node => {
    if (n.kind === "leaf") {
      if (n.pane.id === x) return leaf(py);
      if (n.pane.id === y) return leaf(px);
      return n;
    }
    return { ...n, a: walk(n.a), b: walk(n.b) };
  };
  return walk(node);
}

const easeOutCubic = (t: number) => 1 - Math.pow(1 - t, 3);
const lerp = (from: number, to: number, t: number) => Math.round(from + (to - from) * t);

export function Session({
  theme,
  borderStyle = "sharp",
  alertTab = false,
  compact = false,
  rows: rowsProp,
  start = ["shell"],
  className,
}: {
  theme: SlatTheme;
  borderStyle?: BorderStyle;
  alertTab?: boolean;
  compact?: boolean;
  /** How many rows tall, status bar included. */
  rows?: number;
  /** Which pane bodies the session opens with, left to right. */
  start?: string[];
  className?: string;
}) {
  const [tree, setTree] = useState<Node>(() => initialTree(start));
  const [activeId, setActiveId] = useState(1);
  const [nextId, setNextId] = useState(start.length + 1);
  const [cols, setCols] = useState(88);
  const [anim, setAnim] = useState<Anim>(null);
  const [now, setNow] = useState(0);

  const wrapRef = useRef<HTMLDivElement>(null);
  const gridRef = useRef<HTMLDivElement>(null);
  const probeRef = useRef<HTMLSpanElement>(null);
  const reduced = useRef(false);

  useEffect(() => {
    reduced.current = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }, []);

  // A terminal is as wide as it is: measure one character and fit the grid
  // to the box, then follow it when the window changes — the browser's
  // version of the resize slat gets from the terminal.
  useEffect(() => {
    const measure = () => {
      const probe = probeRef.current;
      const grid = gridRef.current;
      if (!probe || !grid) return;
      const charWidth = probe.getBoundingClientRect().width / 10;
      const room = grid.getBoundingClientRect().width;
      if (!charWidth || !room) return;
      // One column short of the room, so a rounding error can never make
      // the last column wrap and shear the whole grid.
      const fit = Math.floor(room / charWidth) - 1;
      setCols(Math.max(40, Math.min(132, fit)));
    };
    measure();
    const observer = new ResizeObserver(measure);
    if (gridRef.current) observer.observe(gridRef.current);
    return () => observer.disconnect();
  }, []);

  // One frame loop, running only while something is moving.
  useEffect(() => {
    if (!anim) return;
    let raf = 0;
    const tick = () => {
      setNow(performance.now());
      if (performance.now() - anim.start < anim.ms) raf = requestAnimationFrame(tick);
      else setAnim(null);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [anim]);

  const rows = rowsProp ?? (compact ? 14 : ROWS);
  const area: Rect = useMemo(
    () => ({ row: 0, col: 0, rows: rows - 1, cols }),
    [cols, rows]
  );
  const { rects, borders } = useMemo(() => layout(tree, area), [tree, area]);
  const livePanes = useMemo(() => panes(tree), [tree]);

  const split = useCallback(
    (dir: "v" | "h") => {
      if (livePanes.length >= 4) return;
      const id = nextId;
      const name = ["build", "logs", "agent"][(id - 2) % 3];
      setTree((t) =>
        replaceLeaf(t, activeId, (pane) => ({
          kind: "split",
          dir,
          ratio: 0.5,
          a: leaf(pane),
          b: leaf(makePane(id, name, name === "build" ? "working" : "idle")),
        }))
      );
      setNextId(id + 1);
      setActiveId(id);
      if (!reduced.current) {
        setAnim({ kind: "reveal", paneId: id, sideways: dir === "v", start: performance.now(), ms: REVEAL_MS });
      }
    },
    [activeId, livePanes.length, nextId]
  );

  const move = useCallback(
    (direction: "left" | "right") => {
      const here = rects.get(activeId);
      if (!here) return;
      // The neighbour on that side, found the way SelectPaneInDirection
      // does: nearest pane entirely to the left or right that shares a row.
      let best: { id: number; rect: Rect } | null = null;
      for (const pane of livePanes) {
        if (pane.id === activeId) continue;
        const r = rects.get(pane.id);
        if (!r) continue;
        const sharesRow = r.row < here.row + here.rows && here.row < r.row + r.rows;
        if (!sharesRow) continue;
        const isLeft = r.col + r.cols <= here.col;
        const isRight = r.col >= here.col + here.cols;
        if (direction === "left" ? !isLeft : !isRight) continue;
        const gap = direction === "left" ? here.col - (r.col + r.cols) : r.col - (here.col + here.cols);
        if (!best || gap < Math.abs(best.rect.col - here.col)) best = { id: pane.id, rect: r };
      }
      if (!best) return;
      const swapped = swapPanes(tree, activeId, best.id);
      const after = layout(swapped, area).rects;
      setTree(swapped);
      if (!reduced.current) {
        setAnim({
          kind: "slide",
          a: { id: activeId, from: here, to: after.get(activeId) ?? here },
          b: { id: best.id, from: best.rect, to: after.get(best.id) ?? best.rect },
          start: performance.now(),
          ms: SLIDE_MS,
        });
      }
    },
    [activeId, area, livePanes, rects, tree]
  );

  const close = useCallback(() => {
    if (livePanes.length === 1) return;
    const remaining = removeLeaf(tree, activeId);
    if (!remaining) return;
    setTree(remaining);
    setActiveId(panes(remaining)[0].id);
    setAnim(null);
  }, [activeId, livePanes.length, tree]);

  // Compose the frame: panes, then borders, then the bar — the order
  // render() uses.
  const grid = useMemo(() => {
    const g = newGrid(cols, rows);
    const progress = anim ? easeOutCubic(Math.min(1, (now - anim.start) / anim.ms)) : 1;

    const sliding = anim?.kind === "slide" ? [anim.a.id, anim.b.id] : [];
    for (const pane of livePanes) {
      const rect = rects.get(pane.id);
      if (!rect) continue;
      if (anim?.kind === "slide" && sliding.includes(pane.id)) {
        const side = anim.a.id === pane.id ? anim.a : anim.b;
        blit(g, { ...rect, row: lerp(side.from.row, side.to.row, progress), col: lerp(side.from.col, side.to.col, progress) }, pane.lines);
        continue;
      }
      blit(g, rect, pane.lines);
    }

    const activeRect = rects.get(activeId) ?? null;
    drawBorders(g, borders, activeRect);

    if (anim?.kind === "reveal") {
      const rect = rects.get(anim.paneId);
      if (rect) {
        const span = anim.sideways ? rect.cols : rect.rows;
        curtain(g, rect, anim.sideways, Math.round(progress * span));
      }
    }

    const tabs = [{ name: "shell", active: true, alert: alertTab }];
    statusBar(g, rows - 1, {
      workspace: "main",
      tabs,
      pane: Math.max(1, livePanes.findIndex((p) => p.id === activeId) + 1),
      panes: livePanes.length,
      badge: anim ? undefined : undefined,
    });
    return g;
  }, [activeId, alertTab, anim, borders, cols, livePanes, now, rects, rows]);

  const glyphs = BORDER_SETS[borderStyle];
  const remap = (ch: string) => {
    const i = BORDER_SETS.sharp.indexOf(ch);
    return i >= 0 ? glyphs[i] : ch;
  };

  const colorFor = (style: CellStyle): { color: string; background?: string; weight?: number } => {
    switch (style) {
      case "dim":
        return { color: theme.dim };
      case "border":
        return { color: theme.border };
      case "borderActive":
        return { color: theme.accent, weight: 700 };
      case "bar":
        return { color: theme.fg, background: theme.bg };
      case "barDim":
        return { color: theme.dim, background: theme.bg };
      case "workspace":
        return { color: theme.accent, background: theme.bg, weight: 700 };
      case "tab":
        return { color: theme.tabFg, background: theme.tabBg };
      case "tabActive":
        return { color: theme.tabActiveFg, background: theme.tabActiveBg, weight: 700 };
      case "badge":
        return { color: theme.bg, background: theme.accent, weight: 700 };
      case "curtain":
        return { color: theme.border };
      case "prompt":
        return { color: theme.accent };
      case "cursor":
        return { color: theme.fg };
      default:
        return { color: theme.fg };
    }
  };

  const lines = [];
  for (let y = 0; y < rows; y++) {
    const runs: { text: string; style: CellStyle }[] = [];
    for (let x = 0; x < cols; x++) {
      const cell = cellAt(grid, x, y);
      const last = runs[runs.length - 1];
      const ch = remap(cell.ch);
      if (last && last.style === cell.style) last.text += ch;
      else runs.push({ text: ch, style: cell.style });
    }
    lines.push(
      <div key={y} className="whitespace-pre">
        {runs.map((run, i) => {
          const { color, background, weight } = colorFor(run.style);
          return (
            <span
              key={i}
              className={run.style === "cursor" ? "cursor-blink" : undefined}
              style={{ color, background, fontWeight: weight }}
            >
              {run.text}
            </span>
          );
        })}
      </div>
    );
  }

  return (
    <div className={["min-w-0", className].filter(Boolean).join(" ")}>
      <div
        ref={wrapRef}
        className="w-full min-w-0 overflow-hidden rounded-xl p-3 sm:p-4"
        style={{ background: theme.bg, boxShadow: "inset 0 0 0 1px var(--hairline)" }}
      >
        <span
          ref={probeRef}
          aria-hidden
          className="pointer-events-none absolute -z-10 font-mono opacity-0"
          style={{ fontSize: "var(--cell-size)" }}
        >
          0123456789
        </span>
        <div
          ref={gridRef}
          className="font-mono leading-[1.35] tabular-nums"
          style={{ fontSize: "var(--cell-size)" }}
          role="img"
          aria-label={`A slat session with ${livePanes.length} pane${livePanes.length > 1 ? "s" : ""}, ${theme.name} theme`}
        >
          {lines}
        </div>
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2 text-[13px]">
        <Key onClick={() => split("v")} label="v" help="split left / right" />
        <Key onClick={() => split("h")} label="h" help="split top / bottom" />
        <Key onClick={() => move("left")} label="<" help="move pane left" />
        <Key onClick={() => move("right")} label=">" help="move pane right" />
        <Key onClick={close} label="x" help="close pane" />
        <span className="ml-auto font-mono text-ink-faint">
          {cols}×{rows}
        </span>
      </div>
    </div>
  );
}

/** A key you can press: prefix, then this. Styled as the key itself. */
function Key({ label, help, onClick }: { label: string; help: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="liquid group inline-flex items-center gap-2 rounded-full px-3 py-1.5 font-mono text-ink"
    >
      <span className="text-ink">
        <span className="text-ink-muted">C-s</span> {label}
      </span>
      <span className="hidden text-ink-muted sm:inline">{help}</span>
    </button>
  );
}

export { THEMES };
