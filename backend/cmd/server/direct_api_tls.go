package main

import (
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"time"
)

// The optional direct listener shares the existing application and its billing
// state. It never starts a second scheduler or exposes the admin/frontend routes.
func startDirectAPITLS(base *http.Server, forwardedIPHeaders func() []string) *http.Server {
	addr := strings.TrimSpace(os.Getenv("BUAGA_DIRECT_API_TLS_ADDR"))
	if addr == "" {
		return nil
	}
	host := strings.TrimSpace(os.Getenv("BUAGA_DIRECT_API_TLS_HOST"))
	certFile := strings.TrimSpace(os.Getenv("BUAGA_DIRECT_API_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("BUAGA_DIRECT_API_TLS_KEY_FILE"))
	if host == "" || strings.ContainsAny(host, "/:") || certFile == "" || keyFile == "" {
		log.Print("Direct API TLS disabled: host, certificate and key must all be configured")
		return nil
	}

	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Printf("Direct API TLS disabled: load certificate: %v", err)
		return nil
	}
	var current atomic.Pointer[tls.Certificate]
	current.Store(&certificate)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("Direct API TLS disabled: listen on %s: %v", addr, err)
		return nil
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           directAPIHandler(base.Handler, host, forwardedIPHeaders),
		MaxHeaderBytes:    base.MaxHeaderBytes,
		ReadHeaderTimeout: base.ReadHeaderTimeout,
		IdleTimeout:       base.IdleTimeout,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				return current.Load(), nil
			},
		},
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.ServeTLS(listener, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Direct API TLS listener stopped: %v", err)
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				next, err := tls.LoadX509KeyPair(certFile, keyFile)
				if err != nil {
					log.Printf("Direct API TLS certificate reload failed; retaining previous certificate: %v", err)
					continue
				}
				current.Store(&next)
			case <-done:
				return
			}
		}
	}()
	log.Printf("Direct API TLS started on %s for %s", addr, host)
	return server
}

func directAPIHandler(next http.Handler, expectedHost string, forwardedIPHeaders func() []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if parsed, _, err := net.SplitHostPort(host); err == nil {
			host = parsed
		}
		path := r.URL.Path
		if r.TLS == nil || !strings.EqualFold(host, expectedHost) ||
			r.URL.EscapedPath() != path || path != cleanDirectAPIPath(path) ||
			!directAPIAllowedPath(path, r.Method) {
			http.NotFound(w, r)
			return
		}
		// Unlike the main ingress, this socket accepts public peers directly.
		// Never let client-supplied proxy headers replace their network address.
		headers := forwardedIPHeaders()
		for name := range r.Header {
			if isForwardedIPHeader(name, headers) {
				delete(r.Header, name)
			}
		}
		w.Header().Set("Strict-Transport-Security", "max-age=15552000")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func cleanDirectAPIPath(value string) string {
	if strings.HasSuffix(value, "/") {
		return path.Clean(value) + "/"
	}
	return path.Clean(value)
}

func isForwardedIPHeader(name string, custom []string) bool {
	switch strings.ToLower(name) {
	case "forwarded", "x-forwarded-for", "x-real-ip", "x-forwarded-host",
		"x-forwarded-proto", "x-forwarded-port", "x-forwarded-ssl",
		"cf-connecting-ip", "true-client-ip":
		return true
	}
	for _, header := range custom {
		if strings.EqualFold(name, header) {
			return true
		}
	}
	return false
}

func directAPIAllowedPath(path, method string) bool {
	if path == "/health" {
		return method == http.MethodGet || method == http.MethodHead
	}
	for _, prefix := range []string{
		"/v1/", "/v1beta/", "/v3/", "/api/v3/",
		"/backend-api/codex/", "/antigravity/v1/", "/antigravity/v1beta/",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	for _, prefix := range []string{
		"/models", "/responses", "/images", "/videos",
		"/custom-voices", "/contents/generations/tasks", "/antigravity/models",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	switch path {
	case "/chat/completions", "/embeddings", "/messages/count_tokens",
		"/alpha/search", "/tts", "/stt", "/realtime",
		"/web_search", "/x_search":
		return true
	}
	return false
}
