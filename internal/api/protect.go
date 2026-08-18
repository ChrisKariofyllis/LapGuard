package api

import (
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"

	"lapguard/internal/storage"
)

const unauthorizedMessage = "unauthorized"

var localDevOrigins = map[string]struct{}{
	"http://127.0.0.1:5173": {},
	"http://localhost:5173": {},
	"http://127.0.0.1:4173": {},
	"http://localhost:4173": {},
}

func (s *Server) secureWrite(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !s.originAllowed(w, r) {
			return
		}
		if !s.requireJSON(w, r) {
			return
		}
		if !s.requireAuth(w, r) {
			return
		}
		h(w, r)
	}
}

func (s *Server) secureAuthAdmin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !s.originAllowed(w, r) {
			return
		}
		if !s.requireJSON(w, r) {
			return
		}
		cfg := s.currentConfig().Auth
		if cfg.Enabled {
			if !s.requireAuth(w, r) {
				return
			}
		} else if !isLocalConsole(r) {
			s.rejectUnauthorized(w, r)
			return
		}
		h(w, r)
	}
}

func (s *Server) originAllowed(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if originExplicitlyAllowed(origin, r.Host) {
		return true
	}
	s.audit(r, storage.AuditInvalidOrigin, false, "cross-origin request rejected")
	s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin not allowed"})
	return false
}

func originExplicitlyAllowed(origin, host string) bool {
	if _, ok := localDevOrigins[strings.TrimRight(origin, "/")]; ok {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}

func (s *Server) requireJSON(w http.ResponseWriter, r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		s.writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
			"error":  "unsupported media type",
			"detail": "Content-Type must be application/json",
		})
		return false
	}
	media, _, err := mime.ParseMediaType(ct)
	if err != nil || !strings.EqualFold(media, "application/json") {
		s.writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
			"error":  "unsupported media type",
			"detail": "Content-Type must be application/json",
		})
		return false
	}
	return true
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	cfg := s.currentConfig().Auth
	if !cfg.Enabled {
		return true
	}
	token, ok := bearerToken(r)
	if !ok || !cfg.VerifyToken(token) {
		s.rejectUnauthorized(w, r)
		return false
	}
	return true
}

func (s *Server) rejectUnauthorized(w http.ResponseWriter, r *http.Request) {
	s.audit(r, storage.AuditUnauthorized, false, "missing or invalid bearer token")
	w.Header().Set("WWW-Authenticate", `Bearer realm="lapguard"`)
	s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": unauthorizedMessage})
}

func bearerToken(r *http.Request) (string, bool) {
	if r.URL.Query().Get("token") != "" || r.URL.Query().Get("access_token") != "" {
		return "", false
	}
	h := r.Header.Values("Authorization")
	if len(h) != 1 {
		return "", false
	}
	const prefix = "Bearer "
	raw := strings.TrimSpace(h[0])
	if len(raw) < len(prefix) || !strings.EqualFold(raw[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(raw[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

func isLocalConsole(r *http.Request) bool {
	return remoteIsLoopback(r.RemoteAddr) && hostIsLoopback(r.Host)
}

func remoteIsLoopback(remote string) bool {
	host := remote
	if h, _, err := net.SplitHostPort(remote); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func hostIsLoopback(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) audit(r *http.Request, eventType string, success bool, reason string) {
	s.mu.RLock()
	store := s.events
	s.mu.RUnlock()
	if store == nil || r == nil {
		return
	}
	_, _ = store.InsertAudit(r.Context(), storage.AuditEvent{
		Type:     eventType,
		Success:  success,
		Remote:   r.RemoteAddr,
		Method:   r.Method,
		Endpoint: r.URL.Path,
		Reason:   reason,
	})
}
