.PHONY: all

.PHONY: build
build: clean
	go build -o dist/proxytv ./cmd/main.go

.PHONY: linux
linux: clean dist
	GOOS=linux GOARCH=arm64 go build -o dist/proxytv ./cmd/main.go

.PHONY: dist
dist:
	mkdir -p dist

.PHONY: setup
setup:
	go mod download
	go mod tiny

.PHONY: test
test:
	go test ./...

.PHONY: clean
clean:
	rm -rf dist
