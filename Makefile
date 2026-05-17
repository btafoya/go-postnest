.PHONY: build build-server build-webui build-admin build-worker build-migrate run run-webui test migrate admin-setup

build: build-server build-webui build-admin build-worker build-migrate

build-server:
	go build -o postnest-server ./cmd/server

build-webui:
	cd web && npm ci && npm run build
	go build -o postnest-webui ./cmd/webui

build-admin:
	go build -o postnest-admin ./cmd/admin

build-worker:
	go build -o postnest-worker ./cmd/worker

build-migrate:
	go build -o postnest-migrate ./cmd/migrate

run:
	go run ./cmd/server

run-webui:
	go run ./cmd/webui

run-worker:
	go run ./cmd/worker

test:
	go test ./...

migrate:
	go run ./cmd/migrate up

# Create initial admin user + domain
# Usage: make admin-setup EMAIL=admin@example.com PASSWORD=secret DOMAIN=example.com
admin-setup: build-admin
	./admin setup --email $(EMAIL) --password $(PASSWORD) --domain $(DOMAIN)
