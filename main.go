package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultPiholeTarget = "http://127.0.0.1:8080"
	defaultListenAddr   = ":80"
	defaultDBFile       = "go_links.json"

	maxKeyLength    = 50
	maxURLLength    = 2048
	shutdownTimeout = 5 * time.Second
)

var (
	validKeyRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	logger        *slog.Logger
)

// linkStore manages the thread-safe storage of links
type linkStore struct {
	mu    sync.RWMutex
	links map[string]string
}

func newLinkStore() *linkStore {
	return &linkStore{
		links: make(map[string]string),
	}
}

func (s *linkStore) get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, exists := s.links[key]
	return val, exists
}

func (s *linkStore) set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[key] = value
}

func (s *linkStore) getAll() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to avoid external modification
	copy := make(map[string]string, len(s.links))
	for k, v := range s.links {
		copy[k] = v
	}
	return copy
}

func (s *linkStore) load(data map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links = data
}

// config holds application configuration
type config struct {
	piholeTarget string
	listenAddr   string
	dbFile       string
}

func loadConfig() config {
	return config{
		piholeTarget: getEnv("PIHOLE_TARGET", defaultPiholeTarget),
		listenAddr:   getEnv("LISTEN_ADDR", defaultListenAddr),
		dbFile:       getEnv("DB_FILE", defaultDBFile),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg := loadConfig()
	store := newLinkStore()

	// Load existing links
	if err := loadLinks(cfg.dbFile, store); err != nil {
		logger.Error("failed to load links", "error", err)
	}

	// Parse and validate Pi-hole target URL
	targetURL, err := url.Parse(cfg.piholeTarget)
	if err != nil {
		logger.Error("invalid Pi-hole target URL", "url", cfg.piholeTarget, "error", err)
		os.Exit(1)
	}

	// Setup HTTP handlers
	mux := http.NewServeMux()
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		host := strings.Split(r.Host, ":")[0]
		if host == "go" || host == "go.local" {
			handleGo(w, r, store, cfg.dbFile)
		} else {
			proxy.ServeHTTP(w, r)
		}
	})

	// Configure server with reasonable timeouts
	srv := &http.Server{
		Addr:         cfg.listenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("starting server", "addr", cfg.listenAddr, "pihole", cfg.piholeTarget)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	// Save links before shutdown
	if err := saveLinks(cfg.dbFile, store); err != nil {
		logger.Error("failed to save links during shutdown", "error", err)
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}

func handleGo(w http.ResponseWriter, r *http.Request, store *linkStore, dbFile string) {
	path := strings.Trim(r.URL.Path, "/")

	if path == "" {
		if r.Method == http.MethodPost {
			createLink(w, r, store, dbFile)
			return
		}
		showInterface(w, r, store)
		return
	}

	dest, exists := store.get(path)
	if exists {
		logger.Info("redirecting", "key", path, "destination", dest)
		http.Redirect(w, r, dest, http.StatusFound)
	} else {
		logger.Info("link not found", "key", path)
		http.Redirect(w, r, "/?key="+url.QueryEscape(path), http.StatusTemporaryRedirect)
	}
}

func createLink(w http.ResponseWriter, r *http.Request, store *linkStore, dbFile string) {
	key := strings.TrimSpace(r.FormValue("key"))
	urlStr := strings.TrimSpace(r.FormValue("url"))

	// Validate inputs
	if key == "" || urlStr == "" {
		http.Error(w, "Missing key or URL", http.StatusBadRequest)
		return
	}
	if len(key) > maxKeyLength || len(urlStr) > maxURLLength {
		http.Error(w, "Input too long", http.StatusBadRequest)
		return
	}
	if !validKeyRegex.MatchString(key) {
		http.Error(w, "Invalid key characters (use alphanumeric, hyphens, underscores)", http.StatusBadRequest)
		return
	}

	// Validate URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		http.Error(w, "Invalid URL (must be http or https)", http.StatusBadRequest)
		return
	}

	// Store link
	store.set(key, urlStr)

	// Save to disk
	if err := saveLinks(dbFile, store); err != nil {
		logger.Error("failed to save links", "error", err)
		http.Error(w, "Failed to save link", http.StatusInternalServerError)
		return
	}

	logger.Info("created link", "key", key, "url", urlStr)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func showInterface(w http.ResponseWriter, r *http.Request, store *linkStore) {
	preKey := html.EscapeString(r.URL.Query().Get("key"))
	links := store.getAll()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(renderHTML(preKey, links)))
}

func renderHTML(prefilledKey string, links map[string]string) string {
	output := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Go Links</title>
    <meta name="viewport" content="width=device-width,initial-scale=1">
    <style>
        body{font-family:system-ui,-apple-system,sans-serif;max-width:600px;margin:2rem auto;padding:1rem;background:#f4f4f9}
        .card{background:#fff;padding:1.5rem;border-radius:8px;box-shadow:0 2px 5px #0000001a;margin-bottom:1.5rem}
        input{width:100%;padding:10px;margin:5px 0 15px;border:1px solid #ddd;border-radius:4px;box-sizing:border-box}
        button{background:#007bff;color:#fff;border:none;padding:10px;border-radius:4px;cursor:pointer;width:100%;font-size:1rem}
        ul{list-style:none;padding:0}
        li{padding:10px;border-bottom:1px solid #eee;display:flex;justify-content:space-between;align-items:center}
        a{color:#007bff;text-decoration:none;font-weight:700}
        .key{font-family:monospace;background:#f0f0f0;padding:2px 6px;border-radius:3px}
    </style>
</head>
<body>
    <h1>🚀 Go Links</h1>
    <div class="card">
        <h2>Create Link</h2>
        <form method="POST" action="/">
            <label>Shortcut</label>
            <input name="key" value="` + prefilledKey + `" required placeholder="name" pattern="[a-zA-Z0-9_-]+">
            <label>Destination URL</label>
            <input type="url" name="url" required placeholder="https://...">
            <button>Create</button>
        </form>
    </div>
    <div class="card">
        <h2>Shortcuts</h2>`

	if len(links) == 0 {
		output += `<p style="text-align:center;color:#666">No links yet.</p>`
	} else {
		output += `<ul>`
		for k, v := range links {
			escK := html.EscapeString(k)
			escV := html.EscapeString(v)
			output += fmt.Sprintf(`<li><span>go/<span class="key">%s</span></span><a href="%s" target="_blank">➜</a></li>`, escK, escV)
		}
		output += `</ul>`
	}

	output += `</div>
</body>
</html>`

	return output
}

func loadLinks(dbFile string, store *linkStore) error {
	f, err := os.Open(dbFile)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("no existing links file found, starting fresh")
			return nil
		}
		return fmt.Errorf("open links file: %w", err)
	}
	defer f.Close()

	var links map[string]string
	if err := json.NewDecoder(f).Decode(&links); err != nil && err != io.EOF {
		return fmt.Errorf("decode links: %w", err)
	}

	if links != nil {
		store.load(links)
	}

	return nil
}

func saveLinks(dbFile string, store *linkStore) error {
	f, err := os.Create(dbFile)
	if err != nil {
		return fmt.Errorf("create links file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")

	links := store.getAll()
	if err := encoder.Encode(links); err != nil {
		return fmt.Errorf("encode links: %w", err)
	}

	return nil
}
