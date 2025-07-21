package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// TCPSession holds TCP connection details
type TCPSession struct {
	Conn      net.Conn
	isActive  bool
	WebSocket *websocket.Conn
	LastUsed  time.Time
}

var tcpSessions = make(map[string]*TCPSession)
var tcpSessionsMutex sync.RWMutex

// WebSocket upgrader
var upgraderVNC = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Restrict in production
	},
}

// Handle WebSocket to TCP forwarding
func handleVNCWebSocket(c *gin.Context) {
	address := c.Query("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少设备地址"})
		return
	}
	// Upgrade to WebSocket
	ws, err := upgraderVNC.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer ws.Close()

	host, port, err := net.SplitHostPort(address)
	if err == nil {
		if _, err := fmt.Sscanf(port, "%d", new(int)); err == nil {
			var portNum int
			fmt.Sscanf(port, "%d", &portNum)
			address = fmt.Sprintf("%s:%d", host, portNum+5900)
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "设备地址错误"})
			return
		}
	}
	// Connect to TCP server (replace with your TCP server address)
	tcpConn, err := net.Dial("tcp", address)
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("TCP connection failed: %v", err)))
		return
	}

	// Create session
	sessionID := fmt.Sprintf("tcp_%d", time.Now().Unix())
	session := &TCPSession{
		Conn:      tcpConn,
		isActive:  true,
		WebSocket: ws,
		LastUsed:  time.Now(),
	}

	// Store session
	tcpSessionsMutex.Lock()
	tcpSessions[sessionID] = session
	tcpSessionsMutex.Unlock()
	defer func() {
		tcpSessionsMutex.Lock()
		delete(tcpSessions, sessionID)
		tcpSessionsMutex.Unlock()
		closeTCPSession(session)
	}()

	// Send connection success message
	//ws.WriteMessage(websocket.TextMessage, []byte("Connected to TCP server\r\n"))

	// Start forwarding
	go handleTCPOutput(session)
	handleVNCWebSocketInput(session)
}

// Forward TCP output to WebSocket
func handleTCPOutput(session *TCPSession) {
	buffer := make([]byte, 1024)
	for session.isActive {
		n, err := session.Conn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("TCP read failed: %v", err)
			}
			break
		}
		if n > 0 {
			time.Sleep(10 * time.Millisecond) // Prevent flooding
			if err := session.WebSocket.WriteMessage(websocket.BinaryMessage, buffer[:n]); err != nil {
				fmt.Printf("WebSocket write failed: %v", err)
				break
			}
		}
	}
}

// Forward WebSocket input to TCP
func handleVNCWebSocketInput(session *TCPSession) {
	for session.isActive {
		_, message, err := session.WebSocket.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Printf("WebSocket read error: %v", err)
			}
			break
		}
		session.LastUsed = time.Now()
		if _, err := session.Conn.Write(message); err != nil {
			fmt.Printf("TCP write failed: %v", err)
			break
		}
	}
}

// Close TCP session
func closeTCPSession(session *TCPSession) {
	if session == nil {
		return
	}
	session.isActive = false
	if session.Conn != nil {
		session.Conn.Close()
	}
}
