.PHONY: build test vet install

build:
	go build -o seshagy ./cmd/seshagy

test:
	go test ./...

vet:
	go vet ./...

install:
	go install ./cmd/seshagy
