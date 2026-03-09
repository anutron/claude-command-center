BINARY = ccc
REFRESH_BINARY = ccc-refresh
INSTALL_PATH = /usr/local/bin/$(BINARY)
REFRESH_INSTALL_PATH = /usr/local/bin/$(REFRESH_BINARY)

.PHONY: build test install clean

build:
	go build -o $(BINARY) ./cmd/ccc/
	go build -o $(REFRESH_BINARY) ./cmd/ccc-refresh/
	codesign -s - --identifier "com.ccc.tui" -f $(BINARY)
	codesign -s - --identifier "com.ccc.refresh" -f $(REFRESH_BINARY)

test:
	go test -v ./...

install: build
	ln -sf $$(pwd)/$(BINARY) $(INSTALL_PATH)
	ln -sf $$(pwd)/$(REFRESH_BINARY) $(REFRESH_INSTALL_PATH)

clean:
	rm -f $(BINARY) $(REFRESH_BINARY)
