NVIM ?= nvim

.PHONY: test

test:
	XDG_STATE_HOME=/tmp/slat-nvim-state $(NVIM) --headless -n -u NONE -i NONE -c "set rtp+=slat.nvim" -c "runtime plugin/slat.lua" -c "luafile test.lua" -c "qa!"
