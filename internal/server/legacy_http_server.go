package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// LegacyHTTPServer handles HTTP requests from legacy clients that don't follow HTTP standards strictly
type LegacyHTTPServer struct {
	handler http.Handler
}

// NewLegacyHTTPServer creates a new legacy-compatible HTTP server
func NewLegacyHTTPServer(handler http.Handler) *LegacyHTTPServer {
	return &LegacyHTTPServer{handler: handler}
}

// Serve accepts incoming connections and handles them
func (s *LegacyHTTPServer) Serve(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go s.handleConnection(conn)
	}
}

// handleConnection processes a single connection
func (s *LegacyHTTPServer) handleConnection(conn net.Conn) {
	defer conn.Close()
	
	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	
	// Read the entire request
	reader := bufio.NewReader(conn)
	
	// Read request line
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		// Silently ignore connection errors (client disconnected, etc.)
		return
	}
	
	// FIX: Remove trailing spaces before \r\n
	// The BP_MQX_ETH client sends "GET /path HTTP/1.1 \r\n" with an extra space
	requestLine = strings.TrimRight(requestLine, " \r\n") + "\r\n"
	
	// Debug logging disabled by default
	// log.Printf("[DEBUG] Cleaned request line: %q", requestLine)
	
	// Read headers
	var headerLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if line == "\r\n" || line == "\n" {
			break // End of headers
		}
		headerLines = append(headerLines, line)
	}
	
	// Read body if Content-Length is present
	var body []byte
	for _, line := range headerLines {
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			var cl int
			fmt.Sscanf(line, "Content-Length: %d", &cl)
			if cl > 0 {
				body = make([]byte, cl)
				io.ReadFull(reader, body)
			}
			break
		}
	}
	
	// Reconstruct the HTTP request with cleaned request line
	var buf bytes.Buffer
	buf.WriteString(requestLine)
	for _, line := range headerLines {
		buf.WriteString(line)
	}
	buf.WriteString("\r\n")
	if len(body) > 0 {
		buf.Write(body)
	}
	
	// Parse the cleaned request
	req, err := http.ReadRequest(bufio.NewReader(&buf))
	if err != nil {
		// Silently send error response
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}
	
	// Set RemoteAddr for logging
	req.RemoteAddr = conn.RemoteAddr().String()
	
	// Create a response writer that writes to the connection
	w := &legacyResponseWriter{
		conn:   conn,
		header: make(http.Header),
	}
	
	// Call the handler
	s.handler.ServeHTTP(w, req)
	
	// CRITICAL: Flush the buffered response to the connection
	// This writes headers (with Content-Length) and body
	w.flush()
}

// legacyResponseWriter implements http.ResponseWriter for raw connections
// It buffers the response body to calculate Content-Length before sending headers
type legacyResponseWriter struct {
	conn          net.Conn
	header        http.Header
	statusCode    int
	headerWritten bool
	bodyBuffer    bytes.Buffer
}

func (w *legacyResponseWriter) Header() http.Header {
	return w.header
}

func (w *legacyResponseWriter) WriteHeader(statusCode int) {
	if w.headerWritten {
		return
	}
	w.statusCode = statusCode
	// Don't write headers yet - wait for body to calculate Content-Length
}

func (w *legacyResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	// Buffer the body instead of writing directly
	return w.bodyBuffer.Write(data)
}

// flush writes the complete HTTP response (headers + body) to the connection
// CRITICAL: Everything is buffered and sent in a SINGLE write() call
// The BP_MQX_ETH client has a simple parser that expects the entire response at once
func (w *legacyResponseWriter) flush() error {
	if w.headerWritten {
		return nil
	}
	w.headerWritten = true
	
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	
	// Build the entire response in a buffer
	var response bytes.Buffer
	
	// Write status line
	statusText := http.StatusText(w.statusCode)
	fmt.Fprintf(&response, "HTTP/1.1 %d %s\r\n", w.statusCode, statusText)
	
	// CRITICAL: Add Connection: close header for legacy BP_MQX_ETH clients
	fmt.Fprintf(&response, "Connection: close\r\n")
	
	// CRITICAL: Add Content-Length header (required by BP_MQX_ETH client)
	fmt.Fprintf(&response, "Content-Length: %d\r\n", w.bodyBuffer.Len())
	
	// Write other headers
	for key, values := range w.header {
		for _, value := range values {
			fmt.Fprintf(&response, "%s: %s\r\n", key, value)
		}
	}
	
	// End of headers
	fmt.Fprintf(&response, "\r\n")
	
	// Append body
	response.Write(w.bodyBuffer.Bytes())
	
	// CRITICAL: Send everything in a SINGLE write() call
	// This ensures the BP_MQX_ETH client receives the entire response at once
	_, err := w.conn.Write(response.Bytes())
	return err
}
