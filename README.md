# Gomoku (五子棋) server

Spec: [docs/spec.md](docs/spec.md) (五子棋對戰系統規格).

Play: https://renju.blankmaze.cc

Phase 1 (Go backend API) and phase 2 (Vue frontend) are implemented.

## Run

```sh
(cd web && npm install && npm run build)
go build -o bin/renju-server ./cmd/server
RENJU_ADDR=:8200 RENJU_STATIC_DIR=web/dist ./bin/renju-server
```

Open `http://<host>:8200/`. From another Tailscale device use the dev machine's
Tailscale address or MagicDNS name.

Public URL: https://renju.blankmaze.cc goes through a Cloudflare Tunnel to
`localhost:8200`; run the server with `RENJU_ALLOWED_ORIGINS=renju.blankmaze.cc`
so the WebSocket origin check accepts it.

Frontend development with hot reload: run the Go server as above, then
`cd web && npm run dev` (Vite proxies `/ws` to `localhost:8200`; set
`RENJU_SERVER` to point elsewhere, or `VITE_WS_URL` to bypass the proxy).

Settings are environment variables; see `cmd/server/main.go`.

## Test

```sh
go test -race ./...
(cd web && npm test)
```

## Layout

| Path | What |
| --- | --- |
| `internal/game` | Board, move validation, win/draw detection (freestyle) |
| `internal/engine` | `Engine` interface (GomokuEngine), builtin Minimax + alpha-beta, `Managed` timeout/restart/fallback wrapper |
| `internal/server` | Rooms, join codes, reconnect, AI games, WebSocket protocol (`protocol.go` documents every message) |
| `cmd/server` | Entry point and config |
| `web/` | Vue 3 + Vite client: `src/store.ts` (screens, socket, reconnect), `src/components/` (screens and SVG board) |

WebSocket endpoint: `/ws`. All messages are JSON objects with a `type` field.
