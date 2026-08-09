package traefik_x402

import (
	"bytes"
	"net/http"
	"strings"
	"sync"
)

type limitedResponseWriter struct {
	header   http.Header
	body     bytes.Buffer
	maxBytes int64
	status   int
	written  bool
	tooLarge bool
	mu       sync.Mutex
}

func newLimitedResponseWriter(maxBytes int64) *limitedResponseWriter {
	return &limitedResponseWriter{
		header:   make(http.Header),
		maxBytes: maxBytes,
	}
}

func (w *limitedResponseWriter) Header() http.Header {
	return w.header
}

func (w *limitedResponseWriter) WriteHeader(status int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Interim responses cannot be exposed before settlement. Ignore them and
	// retain the final status code instead.
	if status >= 100 && status < 200 {
		return
	}
	if !w.written {
		w.status = status
		w.written = true
	}
}

func (w *limitedResponseWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.written {
		w.status = http.StatusOK
		w.written = true
	}

	remaining := w.maxBytes - int64(w.body.Len())
	if remaining <= 0 {
		w.tooLarge = true
		return len(data), nil
	}
	if int64(len(data)) > remaining {
		_, _ = w.body.Write(data[:int(remaining)])
		w.tooLarge = true
		return len(data), nil
	}

	_, _ = w.body.Write(data)
	return len(data), nil
}

func (w *limitedResponseWriter) statusCode() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.written {
		return http.StatusOK
	}
	return w.status
}

func (w *limitedResponseWriter) exceededLimit() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tooLarge
}

func (w *limitedResponseWriter) flushTo(destination http.ResponseWriter) {
	for name, values := range w.header {
		destination.Header().Del(name)
		for _, value := range values {
			destination.Header().Add(name, value)
		}
	}
	destination.WriteHeader(w.statusCode())
	_, _ = destination.Write(w.body.Bytes())
}

func exposeHeader(header http.Header, name string) {
	for _, existing := range header.Values("Access-Control-Expose-Headers") {
		for _, part := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(part), name) || strings.TrimSpace(part) == "*" {
				return
			}
		}
	}
	header.Add("Access-Control-Expose-Headers", name)
}
