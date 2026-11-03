# Thin aliases for mise tasks (mise.toml is the single source of truth).
.PHONY: build test vet lint fmt verify ci install

build test vet lint fmt verify ci:
	mise run $@

install:
	go install ./cmd/seshagy
