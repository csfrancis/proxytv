GIT_COMMIT := $(shell git rev-parse --short HEAD)

ifeq ($(RELEASE), 1)
	BINDATA_DEBUG_FLAG :=
else
	BINDATA_DEBUG_FLAG := -debug
endif

.PHONY: all

.PHONY: build
build: clean build-css copy-web-assets
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
	go install github.com/go-bindata/go-bindata/v3/...@v3.1.3

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: test
test: build-css copy-web-assets
	go test -tags debug -v ./...

.PHONY: clean
clean:
	rm -rf dist data

.PHONY: copy-web-assets
copy-web-assets:
	mkdir -p data/static data/templates
	go-bindata -fs -prefix "web/static/" -o ./data/static/bindata.go -pkg static $(BINDATA_DEBUG_FLAG) web/static/...
	go-bindata -fs -prefix "web/templates/" -o ./data/templates/bindata.go -pkg templates $(BINDATA_DEBUG_FLAG) web/templates/...

.PHONY: build-css
build-css:
	tailwindcss -i ./web/static/css/styles.css -o ./web/static/css/output.css --minify

.PHONY: watch-css
watch-css:
	tailwindcss -i ./web/static/css/styles.css -o ./web/static/css/output.css --watch
