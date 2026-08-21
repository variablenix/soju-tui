VERSION ?= dev

.PHONY: build test vet clean linux-amd64 linux-arm64 release

build:
	./scripts/build.sh --target host --version "$(VERSION)"

test:
	go test ./...

vet:
	go vet ./...

linux-amd64:
	./scripts/build.sh --target linux-amd64 --version "$(VERSION)"

linux-arm64:
	./scripts/build.sh --target linux-arm64 --version "$(VERSION)"

release:
	./scripts/build.sh --target all --version "$(VERSION)" --release

clean:
	rm -f soju-tui soju-tui-linux-amd64 soju-tui-linux-arm64 dist/soju-tui dist/soju-tui-linux-amd64 dist/soju-tui-linux-arm64 dist/SHA256SUMS dist/BUILDINFO
