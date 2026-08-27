VERSION ?= dev

.PHONY: build test vet clean linux-amd64 linux-arm64 debs release

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

debs:
	./scripts/build-deb.sh

release:
	./scripts/build.sh --target all --version "$(VERSION)" --release
	./scripts/build-deb.sh

clean:
	rm -f soju-tui soju-tui-linux-amd64 soju-tui-linux-arm64 dist/soju-tui dist/soju-tui-linux-amd64 dist/soju-tui-linux-arm64 dist/soju-tui_*_amd64.deb dist/soju-tui_*_arm64.deb dist/SHA256SUMS dist/BUILDINFO dist/THIRD_PARTY_LICENSES
