package main

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/bensema/gotdx/routes"
)

//go:embed index.html
var assets embed.FS

func main() {
	webMux := http.NewServeMux()
	webMux.HandleFunc("/", handleIndex)
	webMux.HandleFunc("/api/methods", handleMethods)
	webMux.HandleFunc("/api/query", handleQuery)

	macClient := newMACSSEClient()
	defer macClient.Disconnect()
	handler := routes.NewRootHandler(webMux, macClient)

	addr := os.Getenv("GOTDX_WEB_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	log.Printf("gotdx webviewer listening on http://%s", addr)
	if err := http.ListenAndServe(addr, withLogging(handler)); err != nil {
		log.Fatal(err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	content, err := assets.ReadFile("index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(content)
}

func handleMethods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "请求方法不允许")
		return
	}
	writeJSON(w, http.StatusOK, methodDefs)
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "请求方法不允许")
		return
	}

	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}

	resp, err := executeQuery(req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
