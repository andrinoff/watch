BINARY := watch
PORT := 8092

.PHONY: ui build dev-api dev-web test fmt install clean

## Build the frontend into web/dist.
ui:
	cd web && npm ci --no-audit --no-fund && npm run build

## Build the single binary (frontend embedded).
build: ui
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/watch

## Run the server alone, pointed at ~/Movies.
dev-api:
	go run ./cmd/watch -addr 127.0.0.1:$(PORT) -dir "$(HOME)/Movies" -no-dlna

## Vite dev server with /api proxied to :8092.
dev-web:
	cd web && npm run dev

## Run the tests.
test:
	go vet ./...
	go test ./...

fmt:
	gofmt -w ./cmd ./internal ./web/embed.go
	cd web && npx tsc --noEmit

install: clean
	sudo ./deploy/install.sh

clean:
	rm -f $(BINARY)
