package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
)

func jsonRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "127.0.0.1:8585"
	req.RemoteAddr = "127.0.0.1:9"
	return req
}

func remoteJSONRequest(method, path, body string) *http.Request {
	req := jsonRequest(method, path, body)
	req.Host = "example.ts.net"
	req.RemoteAddr = "192.0.2.10:4444"
	return req
}
