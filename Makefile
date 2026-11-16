# 斗拱 Dougong — build entry points (git-bash / linux make)

GO ?= go

.PHONY: web build-linux dev test check

web: ## Build frontend and sync into backend/web/dist (go:embed source)
	cd frontend && npm install && npm run build
	rm -rf backend/web/dist
	cp -r frontend/dist backend/web/dist
	touch backend/web/dist/.gitkeep

build-linux: web ## Cross-compile static linux/amd64 single binary (frontend embedded)
	mkdir -p release
	cd backend && GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "-s -w" -o ../release/dougong-linux-amd64 ./cmd/dougong

dev: ## Frontend dev mode (Vite dev server proxying the Go backend)
	cd frontend && npm run dev

test: ## Full backend test suite
	cd backend && $(GO) test ./...

check: ## go build + vet
	cd backend && $(GO) build ./... && $(GO) vet ./...
