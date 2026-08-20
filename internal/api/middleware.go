package api

import "net/http"

// isLoopback reports whether this request is a direct console call to the
// loopback dashboard (127.0.0.1 / localhost / ::1).
//
// Both the TCP peer and the Host header must be loopback. Tailscale Serve
// connects from 127.0.0.1 but sends a MagicDNS Host, so those requests are
// treated as remote and still need a Bearer token for PUT/POST.
func isLoopback(r *http.Request) bool {
	if r == nil {
		return false
	}
	return remoteIsLoopback(r.RemoteAddr) && hostIsLoopback(r.Host)
}
