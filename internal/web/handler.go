package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/romainlancelot/golink/internal/store"
)

const (
	maxKeyLength = 50
	maxURLLength = 2048
)

var validKeyRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Handler handles all HTTP requests for the go links service.
type Handler struct {
	store  *store.Store
	logger *slog.Logger
}

// NewHandler creates a Handler with the given store and logger.
func NewHandler(s *store.Store, logger *slog.Logger) *Handler {
	return &Handler{store: s, logger: logger}
}

// ServeHTTP routes requests to the appropriate handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")

	switch {
	case path == "":
		h.handleRoot(w, r)
	case strings.HasPrefix(path, "api/"):
		h.handleAPI(w, r, path)
	default:
		h.handleRedirect(w, r, path)
	}
}

func (h *Handler) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.createLink(w, r)
		return
	}
	h.showInterface(w, r)
}

func (h *Handler) handleRedirect(w http.ResponseWriter, r *http.Request, key string) {
	dest, ok := h.store.Get(key)
	if ok {
		h.logger.Info("redirecting", "key", key, "dest", dest)
		http.Redirect(w, r, dest, http.StatusFound)
		return
	}
	h.logger.Info("link not found", "key", key)
	http.Redirect(w, r, "/?key="+url.QueryEscape(key), http.StatusTemporaryRedirect)
}

func (h *Handler) handleAPI(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 3 || parts[2] == "" {
		http.Error(w, "invalid API path", http.StatusBadRequest)
		return
	}

	switch parts[1] {
	case "delete":
		h.deleteLink(w, r, parts[2])
	case "update":
		h.updateLink(w, r, parts[2])
	default:
		http.Error(w, "unknown API action", http.StatusNotFound)
	}
}

func (h *Handler) createLink(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.FormValue("key"))
	rawURL := strings.TrimSpace(r.FormValue("url"))

	if key == "" || rawURL == "" {
		http.Error(w, "missing key or URL", http.StatusBadRequest)
		return
	}
	if len(key) > maxKeyLength || len(rawURL) > maxURLLength {
		http.Error(w, "input too long", http.StatusBadRequest)
		return
	}
	if !validKeyRegex.MatchString(key) {
		http.Error(w, "invalid key (use alphanumeric, hyphens, underscores)", http.StatusBadRequest)
		return
	}
	if err := validateURL(rawURL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.Set(key, rawURL); err != nil {
		h.logger.Error("failed to save link", "error", err)
		http.Error(w, "failed to save link", http.StatusInternalServerError)
		return
	}

	h.logger.Info("created link", "key", key, "url", rawURL)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) deleteLink(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deleted, err := h.store.Delete(key)
	if err != nil {
		h.logger.Error("failed to delete link", "key", key, "error", err)
		http.Error(w, "failed to save changes", http.StatusInternalServerError)
		return
	}
	if !deleted {
		http.Error(w, "link not found", http.StatusNotFound)
		return
	}

	h.logger.Info("deleted link", "key", key)

	if r.Method == http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) updateLink(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawURL := strings.TrimSpace(r.FormValue("url"))
	if rawURL == "" {
		http.Error(w, "missing URL", http.StatusBadRequest)
		return
	}
	if len(rawURL) > maxURLLength {
		http.Error(w, "URL too long", http.StatusBadRequest)
		return
	}
	if err := validateURL(rawURL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	updated, err := h.store.Update(key, rawURL)
	if err != nil {
		h.logger.Error("failed to update link", "key", key, "error", err)
		http.Error(w, "failed to save changes", http.StatusInternalServerError)
		return
	}
	if !updated {
		http.Error(w, "link not found", http.StatusNotFound)
		return
	}

	h.logger.Info("updated link", "key", key, "url", rawURL)

	if r.Method == http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) showInterface(w http.ResponseWriter, r *http.Request) {
	preKey := r.URL.Query().Get("key")
	links := h.store.All()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := renderTemplate(w, preKey, links); err != nil {
		h.logger.Error("failed to render template", "error", err)
	}
}

func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must use http or https scheme")
	}
	return nil
}
