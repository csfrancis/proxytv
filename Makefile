GIT_COMMIT := $(shell git rev-parse --short HEAD)

.PHONY: all

.PHONY: build
build: clean
	go build -tags release -o dist/proxytv -ldflags "-X 'main.gitCommit=$(GIT_COMMIT)'" ./cmd/main.go

.PHONY: linux
linux: clean dist
	GOOS=linux GOARCH=arm64 $(MAKE) build

.PHONY: dist
dist:
	mkdir -p dist

.PHONY: setup
setup:
	go mod download
	go mod tidy

.PHONY: test
test:
	go test -tags debug -v ./...

.PHONY: clean
clean:
	rm -rf dist
