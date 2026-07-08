BINARY    := switch-tester-5000
VERSION   := $(shell cat version.txt | tr -d '[:space:]')
LDFLAGS   := -ldflags "-s -w -X github.com/jmcateer/switch-tester-5000/internal/tui.version=$(VERSION)"

DIST      := dist

.PHONY: all clean linux windows macos

all: linux windows macos

linux:
	mkdir -p $(DIST)
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o $(DIST)/$(BINARY)-linux-amd64        .
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o $(DIST)/$(BINARY)-linux-arm64        .

windows:
	mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o $(DIST)/$(BINARY)-windows-amd64.exe .

macos:
	mkdir -p $(DIST)
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o $(DIST)/$(BINARY)-darwin-amd64      .
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o $(DIST)/$(BINARY)-darwin-arm64      .

clean:
	rm -rf $(DIST)

run:
	go run .
