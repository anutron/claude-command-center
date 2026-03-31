BINARY = ccc
REFRESH_BINARY = ai-cron
INSTALL_PATH = /usr/local/bin/$(BINARY)
REFRESH_INSTALL_PATH = /usr/local/bin/$(REFRESH_BINARY)
MAIN_WORKTREE := $(shell git worktree list --porcelain 2>/dev/null | head -2 | grep '^worktree ' | sed 's/^worktree //')
IS_WORKTREE := $(shell [ "$$(pwd)" != "$(MAIN_WORKTREE)" ] && echo 1 || echo 0)

.PHONY: build test install clean servers servers-gmail

build:
	go build -o $(BINARY) ./cmd/ccc/
	go build -o $(REFRESH_BINARY) ./cmd/ai-cron/
	codesign -s - --identifier "com.ccc.tui" -f $(BINARY)

test:
	go test -v ./...

install: build
ifeq ($(IS_WORKTREE),1)
	@echo "Worktree detected — built locally, skipping /usr/local/bin install"
else
	$(MAKE) servers
	ln -sf $$(pwd)/$(BINARY) $(INSTALL_PATH)
	ln -sf $$(pwd)/$(REFRESH_BINARY) $(REFRESH_INSTALL_PATH)
	ln -sf $$(pwd)/scripts/paused-sessions /usr/local/bin/paused-sessions
	ln -sf $$(pwd)/examples/pomodoro/pomodoro.py /usr/local/bin/ccc-pomodoro
	@mkdir -p $(HOME)/.claude/skills
	@for skill in wind-down wind-up bookmark paused-sessions; do \
		rm -f $(HOME)/.claude/skills/$$skill; \
		ln -sf $$(pwd)/.claude/skills/$$skill $(HOME)/.claude/skills/$$skill; \
	done
	@$(MAKE) restart-daemon
endif

restart-daemon:
	@echo "Restarting daemon..."
	@$(BINARY) daemon stop 2>/dev/null || true
	@sleep 1
	@$(BINARY) daemon start 2>/dev/null || echo "  (daemon will auto-start on next TUI launch)"

servers-gmail:
	cd servers/gmail && npm install && npm run build

servers: servers-gmail

clean:
	rm -f $(BINARY) $(REFRESH_BINARY)
