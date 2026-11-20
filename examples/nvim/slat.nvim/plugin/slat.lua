if vim.g.loaded_slat then
  return
end
vim.g.loaded_slat = true

local slat = require("slat")

vim.keymap.set("n", "<Plug>(SlatSendLine)", slat.send_line, { silent = true, desc = "Slat: send line(s)" })
vim.keymap.set("x", "<Plug>(SlatSendSelection)", slat.send_selection, { silent = true, desc = "Slat: send selection" })
vim.keymap.set("i", "<Plug>(SlatSendLine)", slat.send_line, { silent = true, desc = "Slat: send current line" })
vim.keymap.set("n", "<Plug>(SlatSendOperator)", function()
  vim.o.operatorfunc = "v:lua.require'slat'.send_operator"
  return "g@"
end, { expr = true, silent = true, desc = "Slat: send motion" })

vim.api.nvim_create_user_command("SlatSendLine", function()
  slat.send_line()
end, { desc = "Send current line to slat target" })

vim.api.nvim_create_user_command("SlatSendSelection", function(args)
  if args.range == 2 then
    local lines = vim.api.nvim_buf_get_lines(0, args.line1 - 1, args.line2, false)
    if #lines > 0 then
      local target, err = slat.get_target()
      if not target then
        vim.notify("[slat] " .. err, vim.log.levels.ERROR)
        return
      end
      vim.system({ slat.config.slat_bin, "send", "--enter", target, "--", table.concat(lines, "\n") })
    end
  else
    slat.send_selection()
  end
end, { range = true, desc = "Send visual selection or range to slat target" })

vim.api.nvim_create_user_command("SlatTarget", function(args)
  if args.args ~= "" then
    slat.set_target(args.args)
    return
  end
  local target, err = slat.get_target()
  if target then
    vim.notify("[slat] current target: " .. target)
  else
    vim.notify("[slat] " .. err, vim.log.levels.WARN)
  end
end, { nargs = "?", desc = "Show or set /tmp/slat.sendkeys.target" })
