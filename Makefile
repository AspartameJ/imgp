VERSION ?= dev
GOFLAGS = -ldflags="-X 'gitcode.com/DonaldTom/imgp/internal/version.Version=$(VERSION)'"

.PHONY: all build test lint vet fmt golangci clean release

all: build test lint

build:
	go build $(GOFLAGS) -o imgp .

test:
	go clean -testcache
	go test -cover ./...

lint: vet fmt golangci

vet:
	go vet ./...

fmt:
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "gofmt issues:"; \
		gofmt -l .; \
		exit 1; \
	fi

golangci:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed; skipping (see https://golangci-lint.run)"; \
	fi

clean:
	go clean
	rm -f imgp imgp.exe
	rm -rf bin
	rm -f *.tar *.tar.gz

release: build
	@if [ "$(VERSION)" = "dev" ]; then \
		echo "refusing to tag vdev: pass VERSION=x.y.z"; \
		exit 1; \
	fi
	git tag v$(VERSION)
	@echo "Tag v$(VERSION) created. Push manually: git push origin v$(VERSION)"
