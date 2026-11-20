# slat.nvim

Neovim extension for `slat` that provides:
1. **Seamless split/pane navigation** (`Alt+h/j/k/l` by default, or `Ctrl+h/j/k/l`) between Neovim splits and surrounding `slat` panes.
2. **Send lines, motions, or visual selections** to `/tmp/slat.sendkeys.target` using `slat send`.

## Supported Modes

- **Normal (`n`)**: sends the current line (or `[count]` lines), plus operator-pending motions (`gsip`, `gsj`, etc.).
- **Visual (`v` / `V` / `<C-v>`)**: sends charwise, linewise, or blockwise selections and exits visual mode.
- **Insert (`i`)**: sends the current line without leaving Insert mode.

## Installation (`lazy.nvim`)

```lua
{
  dir = "/path/to/slat/examples/nvim/slat.nvim",
  config = function()
    require("slat").setup({
      target_file = "/tmp/slat.sendkeys.target",
      slat_bin = "slat",
      enter = true,
      navigation = {
        keys = "alt", -- default: Alt+h/j/k/l (matches slat [navigation] keys = "alt")
        -- keys = "ctrl", -- switch to Ctrl+h/j/k/l (set [navigation] keys = "ctrl" in slat config)
      },
      mappings = {
        line = "<leader>sl",
        selection = "<leader>ss",
        operator = "gs",
        insert = "<C-s><C-s>",
      },
    })
  end,
}
```

Pass `mappings = false` to `setup()` if you prefer binding `<Plug>(SlatSendLine)`, `<Plug>(SlatSendSelection)`, and `<Plug>(SlatSendOperator)` yourself.

## Target File (`/tmp/slat.sendkeys.target`)

Write any valid `slat send` target into `/tmp/slat.sendkeys.target`:

```sh
echo "2" > /tmp/slat.sendkeys.target
# or cross-session / workspace / tab:
echo "work:dev/active" > /tmp/slat.sendkeys.target
```

Or set/inspect it from Neovim:

```vim
:SlatTarget work:2
:SlatTarget
```
