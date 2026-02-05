package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"html"
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

// Configuration with environment variable fallbacks
var (
	piholeTarget = getEnv("PIHOLE_TARGET", "http://127.0.0.1:8080")
	listenAddr   = getEnv("LISTEN_ADDR", ":80")
	dbFile       = getEnv("DB_FILE", "go_links.json")
)

const (
	maxKeyLength   = 50
	maxURLLength   = 2048
	shutdownTimeout = 5 * time.Second
)

var (
	// Valid key format: alphanumeric, hyphens, underscores
	validKeyRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	
	store = struct {
		sync.RWMutex
		Links map[string]string
	}{Links: make(map[string]string)}
	
	logger *slog.Logger
)

func main() {
	// Load .env file if it exists
	loadEnvFile(".env")
	
	// Setup structured logging
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	
	// Load existing links from storage
	if err := loadLinks(); err != nil {
		logger.Error("Failed to load links", "error", err)
		// Continue running even if load fails (file might not exist yet)
	}

	// Setup proxy to Pi-hole
	targetURL, err := url.Parse(piholeTarget)
	if err != nil {
		logger.Error("Invalid Pi-hole target URL", "url", piholeTarget, "error", err)
		os.Exit(1)
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Setup HTTP handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		host := strings.Split(r.Host, ":")[0]

		// If domain is "go", handle short link redirection
		if host == "go" || host == "go.local" {
			handleGo(w, r)
		} else {
			// Otherwise, proxy to Pi-hole (transparent proxy)
			proxy.ServeHTTP(w, r)
		}
	})

	// Configure HTTP server with timeouts
	srv := &http.Server{
		Addr:         listenAddr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("Starting Go Links service", "addr", listenAddr, "pihole", piholeTarget)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server gracefully...")

	// Save links before shutdown
	if err := saveLinks(); err != nil {
		logger.Error("Failed to save links during shutdown", "error", err)
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("Server stopped")
}

func handleGo(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	
	// Home page (management interface)
	if path == "" {
		if r.Method == http.MethodPost {
			createLink(w, r)
			return
		}
		showInterface(w, r)
		return
	}

	// Perform redirection
	store.RLock()
	dest, exists := store.Links[path]
	store.RUnlock()

	if exists {
		logger.Info("Redirecting", "key", path, "destination", dest)
		http.Redirect(w, r, dest, http.StatusFound)
	} else {
		// Link not found: suggest creating it
		logger.Info("Link not found, suggesting creation", "key", path)
		http.Redirect(w, r, "/?key="+url.QueryEscape(path), http.StatusTemporaryRedirect)
	}
}

func createLink(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.FormValue("key"))
	urlStr := strings.TrimSpace(r.FormValue("url"))

	// Validate key
	if key == "" {
		http.Error(w, "Key cannot be empty", http.StatusBadRequest)
		return
	}
	if len(key) > maxKeyLength {
		http.Error(w, fmt.Sprintf("Key too long (max %d characters)", maxKeyLength), http.StatusBadRequest)
		return
	}
	if !validKeyRegex.MatchString(key) {
		http.Error(w, "Key must contain only alphanumeric characters, hyphens, and underscores", http.StatusBadRequest)
		return
	}

	// Validate URL
	if urlStr == "" {
		http.Error(w, "URL cannot be empty", http.StatusBadRequest)
		return
	}
	if len(urlStr) > maxURLLength {
		http.Error(w, fmt.Sprintf("URL too long (max %d characters)", maxURLLength), http.StatusBadRequest)
		return
	}
	
	parsedURL, err := url.Parse(urlStr)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		http.Error(w, "Invalid URL (must be http or https)", http.StatusBadRequest)
		return
	}

	// Store the link
	store.Lock()
	store.Links[key] = urlStr
	if err := saveLinks(); err != nil {
		store.Unlock()
		logger.Error("Failed to save links", "error", err)
		http.Error(w, "Failed to save link", http.StatusInternalServerError)
		return
	}
	store.Unlock()

	logger.Info("Created link", "key", key, "url", urlStr)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func showInterface(w http.ResponseWriter, r *http.Request) {
	// Get pre-filled key from query params if present
	preKey := html.EscapeString(r.URL.Query().Get("key"))

	htmlStr := `<!DOCTYPE html>
	<html lang="en">
	<head>
		<meta charset="UTF-8">
		<title>Go Links</title>
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<style>
			body{font-family:-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; max-width:600px; margin:2rem auto; padding:1rem; background:#f4f4f9}
			.card{background:white; padding:1.5rem; border-radius:8px; box-shadow:0 2px 5px rgba(0,0,0,0.1); margin-bottom:1.5rem}
			input{width:100%; padding:10px; margin:5px 0 15px; border:1px solid #ddd; border-radius:4px; box-sizing:border-box; font-size:1rem}
			button{background:#007bff; color:white; border:none; padding:10px; border-radius:4px; cursor:pointer; width:100%; font-size:1rem; font-weight:600}
			button:hover{background:#0056b3}
			ul{list-style:none; padding:0}
			li{padding:10px; border-bottom:1px solid #eee; display:flex; justify-content:space-between; align-items:center}
			li:last-child{border-bottom:none}
			a{color:#007bff; text-decoration:none; font-weight:bold}
			a:hover{text-decoration:underline}
			.link-key{font-family:monospace; background:#f0f0f0; padding:2px 6px; border-radius:3px}
			.error{color:#dc3545; font-size:0.9rem; margin-top:-10px; margin-bottom:10px}
		</style>
	</head>
	<body>
		<h1>🚀 Go Links</h1>
		<div class="card">
			<h2>Create New Link</h2>
			<form method="POST" action="/">
				<label for="key">Shortcut name (e.g., ytb, docs, gh)</label>
				<input id="key" name="key" value="` + preKey + `" required placeholder="name" pattern="[a-zA-Z0-9_-]+" maxlength="50">
				<label for="url">Destination URL</label>
				<input id="url" type="url" name="url" required placeholder="https://..." maxlength="2048">
				<button type="submit">Create Link</button>
			</form>
		</div>
		<div class="card">
			<h2>Active Shortcuts (` + fmt.Sprintf("%d", len(store.Links)) + `)</h2>`
	
	if len(store.Links) == 0 {
		htmlStr += `<p style="color:#666; text-align:center; padding:20px">No links yet. Create your first shortcut above!</p>`
	} else {
		htmlStr += `<ul>`
		store.RLock()
		for k, v := range store.Links {
			escapedKey := html.EscapeString(k)
			escapedURL := html.EscapeString(v)
			htmlStr += fmt.Sprintf(`<li><span>go/<span class="link-key">%s</span></span> <a href="%s" target="_blank" rel="noopener noreferrer">%s →</a></li>`, escapedKey, escapedURL, escapedURL)
		}
		store.RUnlock()
		htmlStr += `</ul>`
	}
	
	htmlStr += `</div></body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(htmlStr))
}

func loadLinks() error {
	f, err := os.Open(dbFile)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, not an error
			logger.Info("No existing links file found, starting fresh")
			return nil
		}
		return fmt.Errorf("failed to open DB file: %w", err)
	}
	defer f.Close()

	store.Lock()
	defer store.Unlock()

	if err := json.NewDecoder(f).Decode(&store.Links); err != nil {
		return fmt.Errorf("failed to decode JSON: %w", err)
	}

	logger.Info("Loaded links from storage", "count", len(store.Links))
	return nil
}

func saveLinks() error {
	f, err := os.Create(dbFile)
	if err != nil {
		return fmt.Errorf("failed to create DB file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	
	store.RLock()
	defer store.RUnlock()
	
	if err := encoder.Encode(store.Links); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// loadEnvFile loads environment variables from a .env file
func loadEnvFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		// .env file is optional, not an error if it doesn't exist
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Parse KEY=VALUE format
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			
			// Remove quotes if present
			value = strings.Trim(value, `"'`)
			
			// Only set if not already in environment
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
}