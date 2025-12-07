package middleware

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
)

// Recovery middleware catches panics and returns HTTP 500
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Log the error message
				log.Printf("[PANIC] Error: %v", err)
				
				// Log the stack trace
				log.Printf("[PANIC] Stack trace:\n%s", debug.Stack())
				
				// Return HTTP 500 Internal Server Error
				http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
			}
		}()
		
		// Call the next handler
		next.ServeHTTP(w, r)
	})
}
