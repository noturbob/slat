local ok, err = xpcall(function()
	local slat = require("slat")

	local tmp = vim.fn.tempname()
	vim.fn.mkdir(tmp, "p")

	local target_file = tmp .. "/slat.sendkeys.target"
	local log_file = tmp .. "/slat_calls.log"
	local mock_bin = tmp .. "/mock-slat.sh"

	vim.fn.writefile({
		"#!/bin/sh",
		'printf "%s\\n" "$*" >> ' .. vim.fn.shellescape(log_file),
	}, mock_bin)
	vim.fn.setfperm(mock_bin, "rwxr-xr-x")

	local function read_calls()
		if vim.fn.filereadable(log_file) ~= 1 then
			return {}
		end
		return vim.fn.readfile(log_file)
	end

	local function clear_calls()
		vim.fn.delete(log_file)
	end

	local function wait_for_calls(min_count)
		local waited = vim.wait(2000, function()
			return #read_calls() >= min_count
		end, 10)
		assert(waited, "timed out waiting for mock slat call (got " .. #read_calls() .. ")")
	end

	-- 1. Setup & target read/write
	slat.setup({
		target_file = target_file,
		slat_bin = mock_bin,
		enter = true,
		navigation = { keys = "alt" },
	})

	local missing_t, missing_err = slat.get_target()
	assert(missing_t == nil and missing_err ~= nil, "expected error for missing target file")

	assert(slat.set_target("work:2"))
	local got_t, got_err = slat.get_target()
	assert(got_t == "work:2", got_err)

	-- 2. Buffer setup for line / count / operator / visual sends
	vim.api.nvim_buf_set_lines(0, 0, -1, false, {
		"echo one",
		"echo two",
		"echo three",
		"alpha beta gamma",
	})

	-- Normal single line via <F3>
	clear_calls()
	vim.api.nvim_win_set_cursor(0, { 1, 0 })
	vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<F3>", true, false, true), "mx", false)
	wait_for_calls(1)
	local calls = read_calls()
	assert(calls[1] == "send --enter work:2 -- echo one", "send_line <F3> got: " .. tostring(calls[1]))

	-- Normal [count] lines via <Plug>(SlatSendLine)
	clear_calls()
	vim.api.nvim_win_set_cursor(0, { 2, 0 })
	vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("2<Plug>(SlatSendLine)", true, false, true), "mx", false)
	wait_for_calls(1)
	calls = read_calls()
	assert(calls[1] == "send --enter work:2 -- echo two" and calls[2] == "echo three", "count send got: " .. vim.inspect(calls))

	-- Operator-pending motion (`'[` .. `']`)
	clear_calls()
	vim.fn.setpos("'[", { 0, 4, 1, 0 })
	vim.fn.setpos("']", { 0, 4, 5, 0 })
	slat.send_operator("char")
	wait_for_calls(1)
	calls = read_calls()
	assert(calls[1] == "send --enter work:2 -- alpha", "send_operator char got: " .. tostring(calls[1]))

	-- Visual linewise selection via <F3>
	clear_calls()
	vim.api.nvim_win_set_cursor(0, { 1, 0 })
	vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("Vj<F3>", true, false, true), "mx", false)
	wait_for_calls(1)
	calls = read_calls()
	assert(calls[1] == "send --enter work:2 -- echo one" and calls[2] == "echo two", "visual line got: " .. vim.inspect(calls))

	-- Visual blockwise selection (<C-v>)
	clear_calls()
	vim.api.nvim_win_set_cursor(0, { 4, 0 })
	vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<C-v>4l<Plug>(SlatSendSelection)", true, false, true), "mx", false)
	wait_for_calls(1)
	calls = read_calls()
	assert(calls[1] == "send --enter work:2 -- alpha", "visual block got: " .. tostring(calls[1]))

	-- 3. Seamless navigation: internal split first, then `slat focus` at edge
	vim.env.SLAT = "/tmp/fake-slat.sock"
	vim.o.splitright = true
	vim.cmd("vsplit")
	local right_win = vim.api.nvim_get_current_win()
	clear_calls()

	-- Move left inside Neovim (should change win, NOT call slat focus)
	slat.navigate("h")
	local left_win = vim.api.nvim_get_current_win()
	assert(left_win ~= right_win, "expected wincmd h to move to left split")
	vim.wait(50, function() return false end)
	assert(#read_calls() == 0, "did not expect slat focus when moving inside nvim splits")

	-- Now at left edge: navigate("h") should call `slat focus left`
	slat.navigate("h")
	wait_for_calls(1)
	calls = read_calls()
	assert(calls[1] == "focus left", "edge navigate got: " .. tostring(calls[1]))

	vim.fn.delete(tmp, "rf")
	print("ALL_NVIM_TESTS_PASSED")
end, debug.traceback)

if not ok then
	io.stderr:write(tostring(err) .. "\n")
	os.exit(1)
end
