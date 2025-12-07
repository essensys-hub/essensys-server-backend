package server

import (
	"bytes"
	"log"
	"net"
)

// LoggingListener wraps a net.Listener to log all connection attempts
type LoggingListener struct {
	net.Listener
}

// NewLoggingListener creates a new logging listener
func NewLoggingListener(inner net.Listener) *LoggingListener {
	return &LoggingListener{Listener: inner}
}

// Accept waits for and returns the next connection, logging the attempt
func (l *LoggingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		log.Printf("[DEBUG] TCP error accepting connection: %v", err)
		return nil, err
	}

	// Only log in debug mode
	// log.Printf("[DEBUG] TCP connection from %s", conn.RemoteAddr())
	return newLoggingConn(conn), nil
}

// loggingConn wraps a net.Conn to log data being read
type loggingConn struct {
	net.Conn
	readBuffer bytes.Buffer
	firstRead  bool
}

// newLoggingConn creates a new logging connection
func newLoggingConn(conn net.Conn) *loggingConn {
	return &loggingConn{
		Conn:      conn,
		firstRead: true,
	}
}

// Read logs the first chunk of data read from the connection (debug mode only)
func (c *loggingConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	
	// Debug logging disabled by default - uncomment for troubleshooting
	// if c.firstRead && n > 0 {
	// 	c.firstRead = false
	// 	c.readBuffer.Write(b[:n])
	// 	log.Printf("[DEBUG] First %d bytes from %s:", n, c.RemoteAddr())
	// 	log.Printf("[DEBUG] Raw data (hex): %x", b[:n])
	// 	log.Printf("[DEBUG] Raw data (string): %q", string(b[:n]))
	// 	lines := bytes.Split(b[:n], []byte("\n"))
	// 	if len(lines) > 0 {
	// 		log.Printf("[DEBUG] First line: %q", string(bytes.TrimSpace(lines[0])))
	// 	}
	// }
	
	return n, err
}

// Close logs when the connection is closed (debug mode only)
func (c *loggingConn) Close() error {
	// Debug logging disabled by default
	// log.Printf("[DEBUG] TCP closing connection from %s", c.RemoteAddr())
	return c.Conn.Close()
}
