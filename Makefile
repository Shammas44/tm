.PHONY: build build-linux test clean

BIN_DIR := bin
BINARY := $(BIN_DIR)/tm
BINARY_LINUX := $(BIN_DIR)/tm-linux-amd64

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BINARY) cmd/tm/main.go

build-linux:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY_LINUX) cmd/tm/main.go

test:
	go test -v ./...

clean:
	rm -rf $(BIN_DIR)
