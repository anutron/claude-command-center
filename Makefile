BINARY = ccc
REFRESH_BINARY = ccc-refresh
INSTALL_PATH = /usr/local/bin/$(BINARY)
REFRESH_INSTALL_PATH = /usr/local/bin/$(REFRESH_BINARY)

.PHONY: build test install clean servers servers-gmail

build:
	go build -o $(BINARY) ./cmd/ccc/
	go build -o $(REFRESH_BINARY) ./cmd/ccc-refresh/
	codesign -s - --identifier "com.ccc.tui" -f $(BINARY)
	codesign -s - --identifier "com.ccc.refresh" -f $(REFRESH_BINARY)

test:
	go test -v ./...

install: build servers
	ln -sf $$(pwd)/$(BINARY) $(INSTALL_PATH)
	ln -sf $$(pwd)/$(REFRESH_BINARY) $(REFRESH_INSTALL_PATH)
	ln -sf $$(pwd)/scripts/paused-sessions /usr/local/bin/paused-sessions
	@mkdir -p $(HOME)/.claude/skills
	@for skill in wind-down wind-up bookmark paused-sessions; do \
		rm -f $(HOME)/.claude/skills/$$skill; \
		ln -sf $$(pwd)/.claude/skills/$$skill $(HOME)/.claude/skills/$$skill; \
	done

servers-gmail:
	cd servers/gmail && npm install && npm run build

servers: servers-gmail

clean:
	rm -f $(BINARY) $(REFRESH_BINARY)
