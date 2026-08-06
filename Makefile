.PHONY: dev dev-backend dev-frontend build test lint fmt sandbox-image

dev:
	$(MAKE) -j2 dev-backend dev-frontend

dev-backend:
	cd backend && go run ./cmd/protean-api

dev-frontend:
	cd frontend && pnpm dev

build:
	cd backend && go build ./...
	cd frontend && pnpm build

test:
	cd backend && go test ./...
	cd frontend && pnpm test

lint:
	cd backend && go vet ./... && test -z "$$(gofmt -l .)"
	cd frontend && pnpm lint

fmt:
	cd backend && gofmt -w .

sandbox-image:
	docker build -t protean-sandbox:dev sandbox-image
