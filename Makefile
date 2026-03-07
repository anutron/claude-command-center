BINARY = ccc
INSTALL_PATH = /usr/local/bin/$(BINARY)

.PHONY: build test install clean

build:
	go build -o $(BINARY) ./cmd/ccc/
	codesign -s - --identifier "com.ccc.tui" -f $(BINARY)

test:
	go test -v ./...

install: build
	ln -sf $$(pwd)/$(BINARY) $(INSTALL_PATH)

clean:
	rm -f $(BINARY)
