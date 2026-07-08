VERSION ?= dev
GOFLAGS = -ldflags="-X 'internal/version.Version=$(VERSION)'"

.PHONY: all build test lint clean vet fmt release

all: build test lint

build:
	go build $(GOFLAGS) -o imgp .

test:
	go clean -testcache
	go test -cover ./...

lint: vet fmt

vet:
	go vet ./...

fmt:
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "gofmt issues:"; \
		gofmt -l .; \
		exit 1; \
	fi

clean:
	go clean
	rm -f imgp imgp.exe

release: build
	git tag v$(VERSION)
	@echo "Tag v$(VERSION) created. Push manually: git push origin v$(VERSION)"
