package server

import "net/http"

// NewHandler serves the WebSocket at /ws, a health check at /healthz, and,
// when staticDir is set, the built Vue app at /.
func NewHandler(h *Hub, staticDir string, originPatterns []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		h.ServeWS(w, r, originPatterns)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	if staticDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(staticDir)))
	}
	return mux
}
