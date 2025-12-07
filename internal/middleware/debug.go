package middleware

import (
	"log"
	"net/http"
	"net/http/httputil"
)

// DebugLogger logs all incoming requests with full details
// This is useful for debugging issues with legacy clients
func DebugLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log the raw request
		dump, err := httputil.DumpRequest(r, true)
		if err != nil {
			log.Printf("[DEBUG] Error dumping request: %v", err)
		} else {
			log.Printf("[DEBUG] Raw request:\n%s", string(dump))
		}

		// Log important headers
		log.Printf("[DEBUG] Protocol: %s", r.Proto)
		log.Printf("[DEBUG] Method: %s", r.Method)
		log.Printf("[DEBUG] URL: %s", r.URL.String())
		log.Printf("[DEBUG] Host: %s", r.Host)
		log.Printf("[DEBUG] RemoteAddr: %s", r.RemoteAddr)
		log.Printf("[DEBUG] Content-Length: %d", r.ContentLength)
		log.Printf("[DEBUG] Transfer-Encoding: %v", r.TransferEncoding)

		// Call next handler
		next.ServeHTTP(w, r)
	})
}
