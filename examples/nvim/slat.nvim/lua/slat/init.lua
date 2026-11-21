local M = {}

M.config = {
	target_file = "/tmp/slat.sendkeys.target",
	slat_bin = "slat",
	enter = true,
	trim_empty_edges = false,
	navigation = {
		keys = "ctrl", -- "alt" (<M-h/j/k/l>), "ctrl" (<C-h/j/k/l>), or false
	},
}

local function notify(msg, level)
	vim.schedule(function()
		vim.notify("[slat] " .. msg, level or vim.log.levels.INFO)
	end)
end

function M.get_target()
	local path = M.config.target_file
	if vim.fn.filereadable(path) ~= 1 then
		return nil, "target file not readable: " .. path
	end
	local lines = vim.fn.readfile(path)
	for _, line in ipairs(lines) do
		local trimmed = vim.trim(line)
		if trimmed ~= "" then
			return trimmed, nil
		end
	end
	return nil, "target file is empty: " .. path
end

function M.set_target(target)
	local trimmed = vim.trim(target or "")
	if trimmed == "" then
		notify("target cannot be empty", vim.log.levels.WARN)
		return false
	end
	local ok, err = pcall(vim.fn.writefile, { trimmed }, M.config.target_file)
	if not ok then
		notify("failed to write target: " .. tostring(err), vim.log.levels.ERROR)
		return false
	end
	notify("target set to " .. trimmed)
	return true
end

local function run_send(text)
	local target, err = M.get_target()
	if not target then
		notify(err, vim.log.levels.ERROR)
		return
	end

	if M.config.trim_empty_edges then
		text = text:gsub("^%s*\n", ""):gsub("\n%s*$", "")
	end
	if text == "" then
		return
	end

	local cmd = { M.config.slat_bin, "send" }
	if M.config.enter then
		table.insert(cmd, "--enter")
	end
	table.insert(cmd, target)
	table.insert(cmd, "--")
	table.insert(cmd, text)

	if vim.system then
		vim.system(cmd, { text = true }, function(res)
			if res.code ~= 0 then
				local detail = vim.trim(res.stderr or "")
				if detail == "" then
					detail = "exit code " .. tostring(res.code)
				end
				notify("send failed (" .. target .. "): " .. detail, vim.log.levels.ERROR)
			end
		end)
		return
	end

	local stderr_chunks = {}
	local job = vim.fn.jobstart(cmd, {
		stderr_buffered = true,
		on_stderr = function(_, data)
			if data then
				for _, line in ipairs(data) do
					if line ~= "" then
						table.insert(stderr_chunks, line)
					end
				end
			end
		end,
		on_exit = function(_, code)
			if code ~= 0 then
				local detail = table.concat(stderr_chunks, " ")
				if detail == "" then
					detail = "exit code " .. tostring(code)
				end
				notify("send failed (" .. target .. "): " .. detail, vim.log.levels.ERROR)
			end
		end,
	})
	if job <= 0 then
		notify("failed to launch " .. M.config.slat_bin, vim.log.levels.ERROR)
	end
end

local function get_region_lines(pos1, pos2, mode)
	if vim.fn.exists("*getregion") == 1 then
		return vim.fn.getregion(pos1, pos2, { type = mode })
	end

	local l1, c1 = pos1[2], pos1[3]
	local l2, c2 = pos2[2], pos2[3]
	if l1 > l2 or (l1 == l2 and c1 > c2) then
		l1, l2 = l2, l1
		c1, c2 = c2, c1
	end

	local lines = vim.api.nvim_buf_get_lines(0, l1 - 1, l2, false)
	if #lines == 0 then
		return {}
	end

	if mode == "V" then
		return lines
	elseif mode == "\22" then
		local left = math.min(c1, c2)
		local right = math.max(c1, c2)
		local out = {}
		for _, line in ipairs(lines) do
			table.insert(out, line:sub(left, right))
		end
		return out
	end

	-- Charwise ("v")
	if #lines == 1 then
		lines[1] = lines[1]:sub(c1, c2)
	else
		lines[1] = lines[1]:sub(c1)
		lines[#lines] = lines[#lines]:sub(1, c2)
	end
	return lines
end

--- Send the current line (or [count] lines in Normal mode).
function M.send_line()
	local count = vim.v.count1
	local row = vim.api.nvim_win_get_cursor(0)[1]
	local lines = vim.api.nvim_buf_get_lines(0, row - 1, row - 1 + count, false)
	if #lines == 0 then
		return
	end
	run_send(table.concat(lines, "\n"))
end

--- Send the active visual selection (charwise, linewise, or blockwise).
function M.send_selection()
	local mode = vim.fn.mode(1)
	local v_mode = mode:sub(1, 1)
	if v_mode ~= "v" and v_mode ~= "V" and v_mode ~= "\22" then
		M.send_line()
		return
	end

	local p1 = vim.fn.getpos("v")
	local p2 = vim.fn.getpos(".")
	local lines = get_region_lines(p1, p2, v_mode)

	local esc = vim.api.nvim_replace_termcodes("<Esc>", true, false, true)
	vim.api.nvim_feedkeys(esc, "nx", false)

	if #lines > 0 then
		run_send(table.concat(lines, "\n"))
	end
end

--- Operatorfunc callback for Normal-mode motions (e.g. `gsip`, `gsj`).
function M.send_operator(motion_type)
	local mode = "v"
	if motion_type == "line" then
		mode = "V"
	elseif motion_type == "block" then
		mode = "\22"
	end
	local p1 = vim.fn.getpos("'[")
	local p2 = vim.fn.getpos("']")
	local lines = get_region_lines(p1, p2, mode)
	if #lines > 0 then
		run_send(table.concat(lines, "\n"))
	end
end

local nav_directions = {
	h = "left",
	j = "down",
	k = "up",
	l = "right",
}

--- Navigate within Neovim splits, or cross into the adjacent slat pane when at an edge.
function M.navigate(dir)
	local slat_dir = nav_directions[dir]
	if not slat_dir then
		return
	end
	local before = vim.api.nvim_get_current_win()
	vim.cmd("wincmd " .. dir)
	if vim.api.nvim_get_current_win() ~= before then
		return
	end
	if not vim.env.SLAT or vim.env.SLAT == "" then
		return
	end
	local cmd = { M.config.slat_bin, "focus", slat_dir }
	if vim.system then
		vim.system(cmd, { text = true })
	else
		vim.fn.jobstart(cmd)
	end
end

function M.setup(opts)
	M.config = vim.tbl_deep_extend("force", M.config, opts or {})

	if opts == nil or opts.mappings ~= false then
		local maps = (opts and opts.mappings) or {}
		local line_key = maps.line or "<F3>"
		local sel_key = maps.selection or "<F3>"
		local op_key = maps.operator or "gs"
		local insert_key = maps.insert or "<F3>"

		vim.keymap.set("n", line_key, M.send_line, { desc = "Slat: send line(s)" })
		vim.keymap.set("x", sel_key, M.send_selection, { desc = "Slat: send selection" })
		vim.keymap.set("n", op_key, function()
			vim.o.operatorfunc = "v:lua.require'slat'.send_operator"
			return "g@"
		end, { expr = true, desc = "Slat: send motion" })
		vim.keymap.set("i", insert_key, function()
			M.send_line()
		end, { desc = "Slat: send current line" })
	end

	local nav = M.config.navigation
	if nav and nav ~= false and nav.keys and nav.keys ~= false and nav.keys ~= "off" then
		local mod = nav.keys == "ctrl" and "C" or "M"
		for _, key in ipairs({ "h", "j", "k", "l" }) do
			local lhs = "<" .. mod .. "-" .. key .. ">"
			local rhs = function()
				M.navigate(key)
			end
			vim.keymap.set("n", lhs, rhs, { silent = true, desc = "Slat/Nvim: navigate " .. nav_directions[key] })
			vim.keymap.set("t", lhs, function()
				vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<C-\\><C-n>", true, false, true), "n", false)
				vim.schedule(function()
					M.navigate(key)
				end)
			end, { silent = true, desc = "Slat/Nvim: navigate " .. nav_directions[key] })
		end
	end
end

return M
