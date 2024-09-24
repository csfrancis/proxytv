.PHONY: all

.PHONY: build
build: clean
	go build -tags release -o dist/proxytv ./cmd/main.go

.PHONY: linux
linux: clean dist
	GOOS=linux GOARCH=arm64 go build -tags release -o dist/proxytv ./cmd/main.go

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
