.PHONY: build test vet install
GO ?= go

build:
	go build -o seshagy ./cmd/seshagy

test:
	go test ./...

vet:
	go vet ./...

install:
	go install ./cmd/seshagy
	@bin_dir="$$($(GO) env GOPATH)/bin"; \
	mkdir -p "$$bin_dir" && \
	cp scripts/seshagy-focus-kill "$$bin_dir"/ && \
	chmod +x "$$bin_dir"/seshagy-focus-kill
