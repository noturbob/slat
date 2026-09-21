/**
 * slat's built-in themes, with the values from internal/ui/theme.go. The
 * xterm-256 indexes in the default theme are spelled out as hex here: 235
 * is #262626, 252 is #d0d0d0, 44 is #00d7d7, and so on.
 *
 * A theme colours what slat draws — the status bar, the pane borders, its
 * overlays. Programs running inside panes keep their own colours, which is
 * why nothing here touches pane text.
 */
export type SlatTheme = {
  name: string;
  bg: string;
  fg: string;
  dim: string;
  accent: string;
  border: string;
  tabBg: string;
  tabFg: string;
  tabActiveBg: string;
  tabActiveFg: string;
};

export const THEMES: SlatTheme[] = [
  {
    name: "default",
    bg: "#262626",
    fg: "#d0d0d0",
    dim: "#a8a8a8",
    accent: "#00d7d7",
    border: "#585858",
    tabBg: "#303030",
    tabFg: "#bcbcbc",
    tabActiveBg: "#005faf",
    tabActiveFg: "#ffffff",
  },
  {
    name: "gruvbox",
    bg: "#3c3836",
    fg: "#ebdbb2",
    dim: "#a89984",
    accent: "#fabd2f",
    border: "#665c54",
    tabBg: "#32302f",
    tabFg: "#d5c4a1",
    tabActiveBg: "#458588",
    tabActiveFg: "#fbf1c7",
  },
  {
    name: "nord",
    bg: "#3b4252",
    fg: "#e5e9f0",
    dim: "#a7b1c2",
    accent: "#88c0d0",
    border: "#4c566a",
    tabBg: "#343b48",
    tabFg: "#d8dee9",
    tabActiveBg: "#5e81ac",
    tabActiveFg: "#eceff4",
  },
  {
    name: "rose-pine",
    bg: "#1f1d2e",
    fg: "#e0def4",
    dim: "#908caa",
    accent: "#ebbcba",
    border: "#403d52",
    tabBg: "#26233a",
    tabFg: "#e0def4",
    tabActiveBg: "#31748f",
    tabActiveFg: "#e0def4",
  },
  {
    name: "mono",
    bg: "#c0c0c0",
    fg: "#000000",
    dim: "#808080",
    accent: "#000000",
    border: "#808080",
    tabBg: "#c0c0c0",
    tabFg: "#000000",
    tabActiveBg: "#000000",
    tabActiveFg: "#c0c0c0",
  },
];

export const BORDER_STYLES = ["sharp", "rounded", "heavy", "double", "dashed", "none"] as const;
export type BorderStyle = (typeof BORDER_STYLES)[number];

/** The eleven glyphs per style, as internal/ui/borders.go defines them. */
export const BORDER_SETS: Record<BorderStyle, string> = {
  //        │ ─ ┌ ┐ └ ┘ ├ ┤ ┬ ┴ ┼
  sharp: "│─┌┐└┘├┤┬┴┼",
  rounded: "│─╭╮╰╯├┤┬┴┼",
  heavy: "┃━┏┓┗┛┣┫┳┻╋",
  double: "║═╔╗╚╝╠╣╦╩╬",
  dashed: "╎╌┌┐└┘├┤┬┴┼",
  none: "           ",
};
