# Kontrol

Kontrol is a terminal UI (TUI) for monitoring Kubernetes pods by context and namespace.
It loads contexts from your kubeconfig, lets you switch context/namespace interactively, and shows pod status/health fields in a compact table.

## Project structure

- `cmd/kontrol/main.go` - application bootstrap/runtime wiring.
- `internal/k8s/` - Kubernetes client and pod-to-row mapping.
- `internal/ui/` - Bubble Tea model, table rendering, and key handling.
- `internal/config/` - persisted selections in `~/.kontrol/config.json`.
- `dist/` - build outputs.

## Kubeconfig assumptions

- Kontrol uses `client-go` default kubeconfig loading rules.
- It reads contexts from your default kubeconfig location (or `KUBECONFIG` if set).
- No in-cluster auth path is implemented; run it from an environment with a valid kubeconfig and cluster access.
- Selected context/namespace are persisted to `~/.kontrol/config.json`.

## Hotkeys

- `C` open context selector
- `N` open namespace selector
- `R` refresh pods now
- when a selector is open, type a lowercase letter to jump to the first matching entry (case-insensitive)
- `↑/↓` (also `←/→`) scroll list / move selector
- `enter` apply selection
- `esc` close selector
- `Q` or `ctrl+c` quit

## Run locally

```bash
go run ./cmd/kontrol
```

## Build locally

```bash
mkdir -p dist
go build -o dist/kontrol ./cmd/kontrol
```

## Cross-compilation (amd64/arm64)

```bash
mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64  go build -o dist/kontrol-linux-amd64      ./cmd/kontrol
CGO_ENABLED=0 GOOS=linux GOARCH=arm64  go build -o dist/kontrol-linux-arm64      ./cmd/kontrol
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o dist/kontrol-darwin-amd64     ./cmd/kontrol
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o dist/kontrol-darwin-arm64     ./cmd/kontrol
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/kontrol-windows-amd64.exe ./cmd/kontrol
```

## Testing

```bash
go test ./...
```

Current automated tests cover core config, k8s mapping/client helpers, and UI model behavior.
