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
    name: "catppuccin",
    bg: "#313244",
    fg: "#cdd6f4",
    dim: "#a6adc8",
    accent: "#cba6f7",
    border: "#45475a",
    tabBg: "#181825",
    tabFg: "#bac2de",
    tabActiveBg: "#89b4fa",
    tabActiveFg: "#1e1e2e",
  },
  {
    name: "tokyo-night",
    bg: "#292e42",
    fg: "#c0caf5",
    dim: "#a9b1d6",
    accent: "#7aa2f7",
    border: "#3b4261",
    tabBg: "#16161e",
    tabFg: "#a9b1d6",
    tabActiveBg: "#7aa2f7",
    tabActiveFg: "#1a1b26",
  },
  {
    name: "dracula",
    bg: "#44475a",
    fg: "#f8f8f2",
    dim: "#a4acd4",
    accent: "#bd93f9",
    border: "#6272a4",
    tabBg: "#282a36",
    tabFg: "#f8f8f2",
    tabActiveBg: "#ff79c6",
    tabActiveFg: "#282a36",
  },
  {
    name: "solarized",
    bg: "#073642",
    fg: "#93a1a1",
    dim: "#839496",
    accent: "#2aa198",
    border: "#586e75",
    tabBg: "#002b36",
    tabFg: "#93a1a1",
    tabActiveBg: "#268bd2",
    tabActiveFg: "#002b36",
  },
  {
    name: "latte",
    bg: "#e6e9ef",
    fg: "#4c4f69",
    dim: "#6c6f85",
    accent: "#8839ef",
    border: "#bcc0cc",
    tabBg: "#dce0e8",
    tabFg: "#5c5f77",
    tabActiveBg: "#1e66f5",
    tabActiveFg: "#eff1f5",
  },
  {
    name: "mono",
    bg: "#ffffff",
    fg: "#000000",
    dim: "#808080",
    accent: "#000000",
    border: "#808080",
    tabBg: "#c0c0c0",
    tabFg: "#000000",
    tabActiveBg: "#000000",
    tabActiveFg: "#ffffff",
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
