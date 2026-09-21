/**
 * A tiny port of how slat actually draws a session, so the page shows the
 * program's own model rather than a picture of it.
 *
 * The real thing (internal/layout, internal/ui) lays panes out in a binary
 * tree of splits, works out which border glyph belongs in each cell from
 * the neighbours that are also border cells, and paints everything into one
 * grid of styled cells before sending the difference to the terminal. This
 * does the same, at a smaller scale, in about a hundred lines.
 */

export type SplitDir = "v" | "h"; // v: left/right, h: top/bottom

export type Rect = { row: number; col: number; rows: number; cols: number };

export type Pane = {
  id: number;
  name: string;
  /** Lines the pane is showing, painted from its top-left corner. */
  lines: string[];
  /** Reported by `slat ls`: what the pane is doing. */
  status: "idle" | "working" | "input" | "exited";
};

export type Node =
  | { kind: "leaf"; pane: Pane }
  | { kind: "split"; dir: SplitDir; ratio: number; a: Node; b: Node };

export type Tab = { name: string; root: Node; activeId: number };

/** Styles a cell can carry. The names match what slat calls them. */
export type CellStyle =
  | "text"
  | "dim"
  | "border"
  | "borderActive"
  | "bar"
  | "barDim"
  | "workspace"
  | "tab"
  | "tabActive"
  | "badge"
  | "curtain"
  | "prompt"
  | "cursor";

export type Cell = { ch: string; style: CellStyle };

export type Grid = { cols: number; rows: number; cells: Cell[] };

const BLANK: Cell = { ch: " ", style: "text" };

export function newGrid(cols: number, rows: number): Grid {
  return { cols, rows, cells: Array.from({ length: cols * rows }, () => BLANK) };
}

function set(g: Grid, col: number, row: number, cell: Cell) {
  if (col < 0 || row < 0 || col >= g.cols || row >= g.rows) return;
  g.cells[row * g.cols + col] = cell;
}

export function cellAt(g: Grid, col: number, row: number): Cell {
  return g.cells[row * g.cols + col] ?? BLANK;
}

/**
 * layout assigns every pane a rectangle and collects the cells the borders
 * run through, the way layout.Apply does: a split spends one column (or
 * row) on the border between its halves.
 */
export function layout(node: Node, area: Rect): { rects: Map<number, Rect>; borders: Rect[] } {
  const rects = new Map<number, Rect>();
  const borders: Rect[] = [];

  const walk = (n: Node, r: Rect) => {
    if (n.kind === "leaf") {
      rects.set(n.pane.id, r);
      return;
    }
    if (n.dir === "v") {
      const first = Math.max(1, Math.round((r.cols - 1) * n.ratio));
      const second = Math.max(1, r.cols - 1 - first);
      walk(n.a, { ...r, cols: first });
      borders.push({ row: r.row, col: r.col + first, rows: r.rows, cols: 1 });
      walk(n.b, { ...r, col: r.col + first + 1, cols: second });
    } else {
      const first = Math.max(1, Math.round((r.rows - 1) * n.ratio));
      const second = Math.max(1, r.rows - 1 - first);
      walk(n.a, { ...r, rows: first });
      borders.push({ row: r.row + first, col: r.col, rows: 1, cols: r.cols });
      walk(n.b, { ...r, row: r.row + first + 1, rows: second });
    }
  };
  walk(node, area);
  return { rects, borders };
}

export function panes(node: Node): Pane[] {
  return node.kind === "leaf" ? [node.pane] : [...panes(node.a), ...panes(node.b)];
}

/** Border glyphs, chosen by which neighbours are border cells — slat's
 *  BorderSet.glyph, "sharp" style. */
const UP = 1,
  DOWN = 2,
  LEFT = 4,
  RIGHT = 8;

function glyph(mask: number): string {
  switch (mask) {
    case UP:
    case DOWN:
    case UP | DOWN:
      return "│";
    case LEFT:
    case RIGHT:
    case LEFT | RIGHT:
      return "─";
    case DOWN | RIGHT:
      return "┌";
    case DOWN | LEFT:
      return "┐";
    case UP | RIGHT:
      return "└";
    case UP | LEFT:
      return "┘";
    case UP | DOWN | RIGHT:
      return "├";
    case UP | DOWN | LEFT:
      return "┤";
    case LEFT | RIGHT | DOWN:
      return "┬";
    case LEFT | RIGHT | UP:
      return "┴";
    case UP | DOWN | LEFT | RIGHT:
      return "┼";
    default:
      return "│";
  }
}

export function drawBorders(g: Grid, borders: Rect[], active: Rect | null) {
  const on = new Set<string>();
  for (const r of borders) {
    for (let y = r.row; y < r.row + r.rows; y++) {
      for (let x = r.col; x < r.col + r.cols; x++) on.add(`${x},${y}`);
    }
  }
  const has = (x: number, y: number) => on.has(`${x},${y}`);
  // The border around the focused pane is the one slat lights up.
  const around = active
    ? {
        row: active.row - 1,
        col: active.col - 1,
        rows: active.rows + 2,
        cols: active.cols + 2,
      }
    : null;
  for (const key of on) {
    const [x, y] = key.split(",").map(Number);
    let mask = 0;
    if (has(x, y - 1)) mask |= UP;
    if (has(x, y + 1)) mask |= DOWN;
    if (has(x - 1, y)) mask |= LEFT;
    if (has(x + 1, y)) mask |= RIGHT;
    const lit =
      around &&
      y >= around.row &&
      y < around.row + around.rows &&
      x >= around.col &&
      x < around.col + around.cols;
    set(g, x, y, { ch: glyph(mask), style: lit ? "borderActive" : "border" });
  }
}

/** blit paints a pane's lines at a position, clipped to its rectangle. */
export function blit(g: Grid, rect: Rect, lines: string[], offset = { row: 0, col: 0 }) {
  for (let y = 0; y < rect.rows && y < lines.length; y++) {
    const line = lines[y] ?? "";
    for (let x = 0; x < rect.cols && x < line.length; x++) {
      const ch = line[x];
      if (ch === "\u0000") continue;
      // The prompt character is the one thing a pane paints in the accent
      // colour, so a reader can find where each command begins; the block
      // at the end of a line is a cursor, and cursors blink.
      const style: CellStyle =
        ch === "█" ? "cursor" : line.startsWith("$ ") && x === 0 ? "prompt" : "text";
      set(g, rect.col + offset.col + x, rect.row + offset.row + y, { ch, style });
    }
  }
}

/** curtain covers the part of a rectangle a new pane hasn't revealed yet —
 *  slat's ui.Curtain, glyph and all. */
export function curtain(g: Grid, rect: Rect, sideways: boolean, revealed: number, ch = "░") {
  for (let y = rect.row; y < rect.row + rect.rows; y++) {
    for (let x = rect.col; x < rect.col + rect.cols; x++) {
      const along = sideways ? x - rect.col : y - rect.row;
      if (along < revealed) continue;
      set(g, x, y, { ch, style: "curtain" });
    }
  }
}

/**
 * statusBar writes slat's own bar, from the same format strings the config
 * uses: left is " {workspace} │ {tabs}", right is
 * "{badge} pane {pane}/{panes} ", and a tab whose pane wants input gets the
 * alert marker.
 */
export function statusBar(
  g: Grid,
  row: number,
  st: {
    workspace: string;
    tabs: { name: string; active: boolean; alert: boolean }[];
    pane: number;
    panes: number;
    badge?: string;
  }
) {
  for (let x = 0; x < g.cols; x++) set(g, x, row, { ch: " ", style: "bar" });

  const write = (col: number, text: string, style: CellStyle) => {
    for (let i = 0; i < text.length; i++) set(g, col + i, row, { ch: text[i], style });
    return col + text.length;
  };

  let x = write(0, ` ${st.workspace} `, "workspace");
  x = write(x, "│ ", "barDim");
  for (const tab of st.tabs) {
    const label = ` ${st.tabs.indexOf(tab) + 1}:${tab.name}${tab.alert ? " ?" : ""} `;
    x = write(x, label, tab.active ? "tabActive" : "tab");
    x = write(x, " ", "bar");
  }

  const right = ` pane ${st.pane}/${st.panes} `;
  const badge = st.badge ? ` ${st.badge} ` : "";
  const start = g.cols - right.length - badge.length;
  if (start > x) {
    const after = write(start, badge, "badge");
    write(after, right, "barDim");
  }
}

/** Pane bodies. Plain, believable terminal output — no invented features. */
export const PANE_CONTENT: Record<string, string[]> = {
  shell: [
    "$ slat ls",
    "*1   idle     1:shell   bash   ~/src/slat",
    " 2   working  1:shell   go     ~/src/slat",
    "",
    "$ git log --oneline -3",
    "8be58f0 Hand the whole look over",
    "35cfb86 Move panes around the layout",
    "c2b9315 slat 1.0: agent control, Windows",
    "",
    "$ █",
  ],
  build: [
    "$ go test -race ./...",
    "ok   slat/internal/vt      1.09s",
    "ok   slat/internal/ui      1.02s",
    "ok   slat/internal/pane    1.10s",
    "ok   slat/internal/layout  1.02s",
    "ok   slat/internal/config  1.03s",
    "ok   slat/internal/daemon  12.1s",
    "ok   slat/internal/app     10.3s",
    "",
    "$ █",
  ],
  logs: [
    "$ tail -f ~/.local/state/slat.log",
    "pane 3 opened   80x23",
    "pane 3 status   working",
    "pane 3 status   input",
    "hook  on_input  notify-send",
    "pane 3 status   idle",
    "pane 3 closed",
    "█",
  ],
  agent: [
    "$ slat pane new --cmd 'make release'",
    " 4   working  1:build   make",
    "$ slat wait 4 --for idle --timeout 10m",
    " 4   idle     1:build   bash",
    "$ echo $?",
    "0",
    "$ slat capture 4 --lines 2",
    "release: 6 artifacts, 2m14s",
    "$ █",
  ],
  ask: [
    "$ ./deploy.sh production",
    "Building image ........ done",
    "Pushing to registry ... done",
    "Draining old pods ..... done",
    "",
    "Overwrite production? [y/N] █",
  ],
};
