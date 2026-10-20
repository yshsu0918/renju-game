# Gomoku (五子棋) server

Spec: 五子棋對戰系統規格 (Claude Doc, https://claude.ai/artifact/SqKE3nnG5Ga4wfqPZWgwzZ).

Phase 1 (Go backend API) and phase 2 (Vue frontend) are implemented.

## Run

```sh
(cd web && npm install && npm run build)
go build -o bin/renju-server ./cmd/server
RENJU_ADDR=:8080 RENJU_STATIC_DIR=web/dist ./bin/renju-server
```

Open `http://<host>:8080/`. From another Tailscale device use the dev machine's
Tailscale address or MagicDNS name.

Frontend development with hot reload: run the Go server as above, then
`cd web && npm run dev` (Vite proxies `/ws` to `localhost:8080`; set
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
