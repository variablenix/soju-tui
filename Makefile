APP := soju-tui
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test vet clean linux-amd64 linux-arm64

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(APP) .

test:
	go test ./...

vet:
	go vet ./...

linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(APP)-linux-amd64 .

linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(APP)-linux-arm64 .

clean:
	rm -f $(APP) $(APP)-linux-amd64 $(APP)-linux-arm64
