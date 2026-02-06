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
	"sort"
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

func (s *linkStore) delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.links[key]; exists {
		delete(s.links, key)
		return true
	}
	return false
}

func (s *linkStore) update(key, newValue string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.links[key]; exists {
		s.links[key] = newValue
		return true
	}
	return false
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

	// Home page with interface
	if path == "" {
		if r.Method == http.MethodPost {
			createLink(w, r, store, dbFile)
			return
		}
		showInterface(w, r, store)
		return
	}

	// API endpoints for link management
	if strings.HasPrefix(path, "api/") {
		handleAPI(w, r, path, store, dbFile)
		return
	}

	// Redirect to destination
	dest, exists := store.get(path)
	if exists {
		logger.Info("redirecting", "key", path, "destination", dest)
		http.Redirect(w, r, dest, http.StatusFound)
	} else {
		logger.Info("link not found", "key", path)
		http.Redirect(w, r, "/?key="+url.QueryEscape(path), http.StatusTemporaryRedirect)
	}
}

func handleAPI(w http.ResponseWriter, r *http.Request, path string, store *linkStore, dbFile string) {
	// Extract key from path (api/delete/key or api/update/key)
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid API path", http.StatusBadRequest)
		return
	}

	action := parts[1]
	key := parts[2]

	switch action {
	case "delete":
		deleteLink(w, r, key, store, dbFile)
	case "update":
		updateLink(w, r, key, store, dbFile)
	default:
		http.Error(w, "Unknown API action", http.StatusNotFound)
	}
}

func deleteLink(w http.ResponseWriter, r *http.Request, key string, store *linkStore, dbFile string) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !store.delete(key) {
		http.Error(w, "Link not found", http.StatusNotFound)
		return
	}

	if err := saveLinks(dbFile, store); err != nil {
		logger.Error("failed to save links after deletion", "error", err)
		http.Error(w, "Failed to save changes", http.StatusInternalServerError)
		return
	}

	logger.Info("deleted link", "key", key)

	if r.Method == http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func updateLink(w http.ResponseWriter, r *http.Request, key string, store *linkStore, dbFile string) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	urlStr := strings.TrimSpace(r.FormValue("url"))
	if urlStr == "" {
		http.Error(w, "Missing URL", http.StatusBadRequest)
		return
	}

	if len(urlStr) > maxURLLength {
		http.Error(w, "URL too long", http.StatusBadRequest)
		return
	}

	// Validate URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		http.Error(w, "Invalid URL (must be http or https)", http.StatusBadRequest)
		return
	}

	if !store.update(key, urlStr) {
		http.Error(w, "Link not found", http.StatusNotFound)
		return
	}

	if err := saveLinks(dbFile, store); err != nil {
		logger.Error("failed to save links after update", "error", err)
		http.Error(w, "Failed to save changes", http.StatusInternalServerError)
		return
	}

	logger.Info("updated link", "key", key, "url", urlStr)

	if r.Method == http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	} else {
		w.WriteHeader(http.StatusNoContent)
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

// sortedKeys returns map keys in alphabetical order
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func renderHTML(prefilledKey string, links map[string]string) string {
	output := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Go Links</title>
    <meta name="viewport" content="width=device-width,initial-scale=1">
    <style>
        body{font-family:system-ui,-apple-system,sans-serif;max-width:900px;margin:2rem auto;padding:1rem;background:#f4f4f9}
        .card{background:#fff;padding:1.5rem;border-radius:8px;box-shadow:0 2px 5px #0000001a;margin-bottom:1.5rem}
        input{width:100%;padding:10px;margin:5px 0 15px;border:1px solid #ddd;border-radius:4px;box-sizing:border-box}
        button{background:#007bff;color:#fff;border:none;padding:10px;border-radius:4px;cursor:pointer;width:100%;font-size:1rem}
        button:hover{background:#0056b3}
        
        table{width:100%;border-collapse:collapse;margin-top:1rem}
        thead{background:#f8f9fa;border-bottom:2px solid #dee2e6}
        th{padding:12px;text-align:left;font-weight:600;color:#495057;font-size:0.9rem;text-transform:uppercase;letter-spacing:0.5px}
        tbody tr{border-bottom:1px solid #dee2e6;transition:background 0.2s}
        tbody tr:hover{background:#f8f9fa}
        td{padding:12px;vertical-align:middle}
        
        .shortcut-cell{font-family:monospace;background:#e9ecef;padding:4px 8px;border-radius:4px;display:inline-block;font-size:0.95rem;font-weight:600;color:#495057}
        .url-cell{color:#6c757d;font-size:0.9rem;max-width:400px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
        .url-cell a{color:#007bff;text-decoration:none;transition:color 0.2s}
        .url-cell a:hover{color:#0056b3;text-decoration:underline}
        .actions-cell{display:flex;gap:8px;justify-content:flex-end}
        
        .btn-edit,.btn-delete{font-size:0.85rem;padding:6px 12px;cursor:pointer;border:1px solid;border-radius:4px;background:transparent;transition:all 0.2s;font-weight:500}
        .btn-edit{color:#28a745;border-color:#28a745}
        .btn-edit:hover{background:#28a745;color:#fff}
        .btn-delete{color:#dc3545;border-color:#dc3545}
        .btn-delete:hover{background:#dc3545;color:#fff}
        
        .edit-row{display:none}
        .edit-row td{background:#f8f9fa;padding:15px}
        .edit-form{display:flex;gap:10px;align-items:center}
        .edit-form input{margin:0;padding:10px;flex:1}
        .edit-form button{width:auto;padding:8px 16px;font-size:0.9rem;margin:0}
        .edit-form .btn-save{background:#28a745;border:none}
        .edit-form .btn-save:hover{background:#218838}
        .edit-form .btn-cancel{background:#6c757d;border:none}
        .edit-form .btn-cancel:hover{background:#5a6268}
        
        .empty-state{text-align:center;padding:3rem 1rem;color:#6c757d}
        .empty-state-icon{font-size:3rem;margin-bottom:1rem;opacity:0.5}
        
        /* Mobile responsive */
        @media (max-width: 768px) {
            body{margin:1rem auto;padding:0.5rem}
            .card{padding:1rem;margin-bottom:1rem}
            h1{font-size:1.5rem;text-align:center}
            h2{font-size:1.2rem}
            
            /* Hide table headers on mobile */
            thead{display:none}
            
            /* Transform table rows into cards */
            tbody tr{
                display:block;
                margin-bottom:1rem;
                border:1px solid #dee2e6;
                border-radius:8px;
                background:#fff;
                box-shadow:0 1px 3px rgba(0,0,0,0.1);
                padding:0;
            }
            
            tbody tr:hover{background:#fff}
            
            td{
                display:block;
                padding:10px 15px;
                border:none;
                text-align:left;
            }
            
            td:before{
                content:attr(data-label);
                font-weight:600;
                color:#495057;
                display:block;
                margin-bottom:5px;
                font-size:0.85rem;
                text-transform:uppercase;
                letter-spacing:0.5px;
            }
            
            .shortcut-cell{
                display:block;
                width:fit-content;
                font-size:1rem;
                padding:6px 10px;
            }
            
            .url-cell{
                max-width:100%;
                white-space:normal;
                word-break:break-all;
                font-size:0.85rem;
            }
            
            .actions-cell{
                justify-content:stretch;
                padding:10px 15px 15px;
            }
            
            .actions-cell:before{display:none}
            
            .btn-edit,.btn-delete{
                flex:1;
                text-align:center;
                padding:10px;
                font-size:0.9rem;
            }
            
            /* Edit form on mobile */
            .edit-row{
                display:none;
                border:none;
                margin-bottom:1rem;
                padding:0;
            }
            
            .edit-row td{
                padding:15px;
                background:#f8f9fa;
                border-radius:8px;
            }
            
            .edit-form{
                flex-direction:column;
                gap:10px;
            }
            
            .edit-form strong{
                display:block;
                margin-bottom:5px;
                font-size:0.9rem;
                color:#495057;
            }
            
            .edit-form input{
                width:100%;
            }
            
            .edit-form button{
                width:100%;
                padding:10px;
            }
        }
        
        @media (max-width: 480px) {
            body{padding:0.25rem}
            .card{padding:0.75rem;border-radius:6px}
            h1{font-size:1.3rem}
            h2{font-size:1.1rem}
            input{padding:8px;font-size:0.95rem}
            button{padding:8px;font-size:0.95rem}
        }
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
		output += `<div class="empty-state">
            <div class="empty-state-icon">🔗</div>
            <p>No links yet. Create your first shortcut above!</p>
        </div>`
	} else {
		output += `<table>
            <thead>
                <tr>
                    <th>Shortcut</th>
                    <th>Destination</th>
                    <th style="text-align:right">Actions</th>
                </tr>
            </thead>
            <tbody>`
		
		sortedKeys := sortedKeys(links)
		for _, k := range sortedKeys {
			v := links[k]
			escK := html.EscapeString(k)
			escV := html.EscapeString(v)
			output += fmt.Sprintf(`
                <tr id="row-%s">
                    <td data-label="Shortcut"><span class="shortcut-cell">go/%s</span></td>
                    <td data-label="Destination" class="url-cell"><a href="%s" target="_blank" title="%s">%s</a></td>
                    <td data-label="Actions" class="actions-cell">
                        <button class="btn-edit" onclick="editLink('%s')">✏️ Edit</button>
                        <button class="btn-delete" onclick="deleteLink('%s')">🗑️ Delete</button>
                    </td>
                </tr>
                <tr id="edit-%s" class="edit-row">
                    <td colspan="3">
                        <form class="edit-form" onsubmit="return updateLink('%s', event)">
                            <strong>go/%s</strong>
                            <input type="url" name="url" value="%s" required placeholder="https://...">
                            <button type="submit" class="btn-save">💾 Save</button>
                            <button type="button" class="btn-cancel" onclick="cancelEdit('%s')">✖️ Cancel</button>
                        </form>
                    </td>
                </tr>`, escK, escK, escV, escV, escV, escK, escK, escK, escK, escK, escV, escK)
		}
		output += `
            </tbody>
        </table>`
	}

	output += `    </div>
    <script>
        function editLink(key) {
            const row = document.getElementById('row-' + key);
            const editRow = document.getElementById('edit-' + key);
            row.style.display = 'none';
            editRow.style.display = 'table-row';
        }
        
        function cancelEdit(key) {
            const row = document.getElementById('row-' + key);
            const editRow = document.getElementById('edit-' + key);
            row.style.display = 'table-row';
            editRow.style.display = 'none';
        }
        
        function updateLink(key, event) {
            event.preventDefault();
            const form = event.target;
            const url = form.url.value;
            const formData = new FormData();
            formData.append('url', url);
            
            fetch('/api/update/' + encodeURIComponent(key), {
                method: 'POST',
                body: formData
            })
            .then(response => {
                if (response.ok) {
                    window.location.reload();
                } else {
                    alert('Failed to update link');
                }
            })
            .catch(error => {
                alert('Error: ' + error);
            });
            return false;
        }
        
        function deleteLink(key) {
            if (!confirm('Delete link "' + key + '"?')) return;
            
            fetch('/api/delete/' + encodeURIComponent(key), {
                method: 'POST'
            })
            .then(response => {
                if (response.ok) {
                    window.location.reload();
                } else {
                    alert('Failed to delete link');
                }
            })
            .catch(error => {
                alert('Error: ' + error);
            });
        }
    </script>
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
