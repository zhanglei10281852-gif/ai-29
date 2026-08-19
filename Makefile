GO ?= go

.PHONY: test race vet build run seed-user

test:
	$(GO) test ./... -count=1

race:
	$(GO) test -race ./... -count=1

vet:
	$(GO) vet ./...

build:
	$(GO) build ./...

run:
	$(GO) run ./cmd/server

seed-user:
	$(GO) run ./cmd/seed-user
