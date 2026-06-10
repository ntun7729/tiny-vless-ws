package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	uuidBytes [16]byte
	wsPath    = "/vless"
)

func main() {
	uuidStr := os.Getenv("UUID")
	if uuidStr == "" {
		log.Fatalf("UUID environment variable is required")
	}

	var err error
	uuidBytes, err = parseUUID(uuidStr)
	if err != nil {
		log.Fatalf("Invalid UUID format: %v", err)
	}

	if envPath := os.Getenv("PATH"); envPath != "" {
		wsPath = envPath
		if !strings.HasPrefix(wsPath, "/") {
			wsPath = "/" + wsPath
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", handleHome)
	http.HandleFunc(wsPath, handleVLESSUpgrade)

	log.Printf("Starting tiny-vless-ws server (zero-dependency)...")
	log.Printf("Path: %s", wsPath)
	log.Printf("Listening on port: %s", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("ListenAndServe error: %v", err)
	}
}

func parseUUID(s string) ([16]byte, error) {
	var uuid [16]byte
	s = strings.ReplaceAll(s, "-", "")
	if len(s) != 32 {
		return uuid, fmt.Errorf("UUID must be 36 characters (or 32 hex chars)")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return uuid, err
	}
	copy(uuid[:], b)
	return uuid, nil
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, "Web server is active")
}

func handleVLESSUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "Unsupported transport mechanism", http.StatusBadRequest)
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Sec-WebSocket-Key missing", http.StatusBadRequest)
		return
	}

	// Calculate WebSocket accept key
	hash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	acceptKey := base64.StdEncoding.EncodeToString(hash[:])

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Websocket hijacking not supported", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		log.Printf("Hijack failed: %v", err)
		return
	}
	defer conn.Close()

	// Write switching protocols response
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"

	_, err = bufrw.WriteString(response)
	if err != nil {
		return
	}
	bufrw.Flush()

	ws := &wsConn{
		conn:   conn,
		bufrd:  bufrw.Reader,
		bufwr:  bufrw.Writer,
		reader: nil,
	}

	// Read first message (VLESS header + initial payload)
	firstMsg, err := ws.NextMessage()
	if err != nil {
		log.Printf("[WS] Failed to read first message: %v", err)
		return
	}

	if len(firstMsg) < 18 {
		log.Printf("[VLESS] Header too short")
		return
	}

	// 1. Version (1B)
	version := firstMsg[0]
	if version != 0 {
		log.Printf("[VLESS] Unsupported VLESS version: %d", version)
		return
	}

	// 2. UUID (16B)
	var clientID [16]byte
	copy(clientID[:], firstMsg[1:17])
	if clientID != uuidBytes {
		log.Printf("[VLESS] Authentication failed: received %s", hex.EncodeToString(clientID[:]))
		return
	}

	// 3. Addon length (1B)
	addonLen := int(firstMsg[17])
	if len(firstMsg) < 18+addonLen {
		log.Printf("[VLESS] Payload too short for addons")
		return
	}

	// 4. Command (1B)
	cmdIdx := 18 + addonLen
	if len(firstMsg) <= cmdIdx {
		log.Printf("[VLESS] Payload too short: no command")
		return
	}
	command := firstMsg[cmdIdx]

	// 5. Port (2B)
	if len(firstMsg) < cmdIdx+3 {
		log.Printf("[VLESS] Payload too short: no port")
		return
	}
	port := binary.BigEndian.Uint16(firstMsg[cmdIdx+1 : cmdIdx+3])

	// 6. Address Type (1B)
	addrTypeIdx := cmdIdx + 3
	if len(firstMsg) <= addrTypeIdx {
		log.Printf("[VLESS] Payload too short: no address type")
		return
	}
	addrType := firstMsg[addrTypeIdx]

	// 7. Address
	var targetAddr string
	var addrEnd int

	switch addrType {
	case 1: // IPv4 (4B)
		if len(firstMsg) < addrTypeIdx+5 {
			log.Printf("[VLESS] Payload too short for IPv4 address")
			return
		}
		targetAddr = net.IP(firstMsg[addrTypeIdx+1 : addrTypeIdx+5]).String()
		addrEnd = addrTypeIdx + 5
	case 2: // Domain name (1B length + domain)
		if len(firstMsg) < addrTypeIdx+2 {
			log.Printf("[VLESS] Payload too short for Domain length")
			return
		}
		domainLen := int(firstMsg[addrTypeIdx+1])
		if len(firstMsg) < addrTypeIdx+2+domainLen {
			log.Printf("[VLESS] Payload too short for Domain name")
			return
		}
		targetAddr = string(firstMsg[addrTypeIdx+2 : addrTypeIdx+2+domainLen])
		addrEnd = addrTypeIdx + 2 + domainLen
	case 3: // IPv6 (16B)
		if len(firstMsg) < addrTypeIdx+17 {
			log.Printf("[VLESS] Payload too short for IPv6 address")
			return
		}
		targetAddr = net.IP(firstMsg[addrTypeIdx+1 : addrTypeIdx+17]).String()
		addrEnd = addrTypeIdx + 17
	default:
		log.Printf("[VLESS] Unknown address type: %d", addrType)
		return
	}

	initialPayload := firstMsg[addrEnd:]

	wsStreamRd := &wsConnStreamReader{ws: ws, reader: bytes.NewReader(initialPayload)}
	wsStreamWr := &wsConnStreamWriter{ws: ws}

	if command == 1 { // TCP
		handleTCP(wsStreamRd, wsStreamWr, targetAddr, port)
	} else if command == 2 { // UDP
		handleUDP(wsStreamRd, wsStreamWr, targetAddr, port)
	} else {
		log.Printf("[VLESS] Unsupported command: %d", command)
	}
}

func handleTCP(wsReader io.Reader, wsWriter io.Writer, addr string, port uint16) {
	dest := net.JoinHostPort(addr, strconv.Itoa(int(port)))
	log.Printf("[TCP] Dialing to %s", dest)

	targetConn, err := net.DialTimeout("tcp", dest, 10*time.Second)
	if err != nil {
		log.Printf("[TCP] Dial target failed: %v", err)
		return
	}
	defer targetConn.Close()

	// Send VLESS response: Version (0x00) + Addons length (0x00)
	if _, err := wsWriter.Write([]byte{0, 0}); err != nil {
		log.Printf("[TCP] Send response header failed: %v", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Forward: WS -> Target
	go func() {
		defer wg.Done()
		defer targetConn.Close()
		_, _ = io.Copy(targetConn, wsReader)
	}()

	// Forward: Target -> WS
	go func() {
		defer wg.Done()
		defer targetConn.Close()
		_, _ = io.Copy(wsWriter, targetConn)
	}()

	wg.Wait()
	log.Printf("[TCP] Connection to %s closed", dest)
}

func handleUDP(wsReader io.Reader, wsWriter io.Writer, addr string, port uint16) {
	dest := net.JoinHostPort(addr, strconv.Itoa(int(port)))
	log.Printf("[UDP] Dialing to %s", dest)

	targetConn, err := net.Dial("udp", dest)
	if err != nil {
		log.Printf("[UDP] Dial target failed: %v", err)
		return
	}
	defer targetConn.Close()

	// Send VLESS response: Version (0x00) + Addons length (0x00)
	if _, err := wsWriter.Write([]byte{0, 0}); err != nil {
		log.Printf("[UDP] Send response header failed: %v", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Forward: WS -> Target UDP
	go func() {
		defer wg.Done()
		defer targetConn.Close()

		lenBuf := make([]byte, 2)
		for {
			_, err := io.ReadFull(wsReader, lenBuf)
			if err != nil {
				return
			}
			pktLen := int(binary.BigEndian.Uint16(lenBuf))
			if pktLen <= 0 || pktLen > 65535 {
				return
			}
			pktBuf := make([]byte, pktLen)
			_, err = io.ReadFull(wsReader, pktBuf)
			if err != nil {
				return
			}
			_, err = targetConn.Write(pktBuf)
			if err != nil {
				return
			}
		}
	}()

	// Forward: Target UDP -> WS
	go func() {
		defer wg.Done()
		defer targetConn.Close()

		buf := make([]byte, 65536)
		for {
			n, err := targetConn.Read(buf)
			if err != nil {
				return
			}
			if n <= 0 {
				continue
			}
			outBuf := make([]byte, 2+n)
			binary.BigEndian.PutUint16(outBuf[0:2], uint16(n))
			copy(outBuf[2:], buf[:n])
			_, err = wsWriter.Write(outBuf)
			if err != nil {
				return
			}
		}
	}()

	wg.Wait()
	log.Printf("[UDP] Connection to %s closed", dest)
}

// wsConn is a custom WebSocket server reader/writer
type wsConn struct {
	conn    net.Conn
	bufrd   *bufio.Reader
	bufwr   *bufio.Writer
	reader  io.Reader
	writeMu sync.Mutex
}

// NextMessage reads the next WebSocket message and returns the payload.
// Filters out Ping, Pong, Close and text messages.
func (ws *wsConn) NextMessage() ([]byte, error) {
	for {
		header, err := ws.bufrd.Peek(2)
		if err != nil {
			return nil, err
		}

		fin := (header[0] & 0x80) != 0
		opcode := header[0] & 0x0f
		masked := (header[1] & 0x80) != 0
		payloadLen := int(header[1] & 0x7f)

		// Consume the 2 header bytes
		_, _ = ws.bufrd.Discard(2)

		var length int64
		if payloadLen == 126 {
			extLenBytes := make([]byte, 2)
			_, err = io.ReadFull(ws.bufrd, extLenBytes)
			if err != nil {
				return nil, err
			}
			length = int64(binary.BigEndian.Uint16(extLenBytes))
		} else if payloadLen == 127 {
			extLenBytes := make([]byte, 8)
			_, err = io.ReadFull(ws.bufrd, extLenBytes)
			if err != nil {
				return nil, err
			}
			length = int64(binary.BigEndian.Uint64(extLenBytes))
		} else {
			length = int64(payloadLen)
		}

		var mask [4]byte
		if masked {
			_, err = io.ReadFull(ws.bufrd, mask[:])
			if err != nil {
				return nil, err
			}
		}

		payload := make([]byte, length)
		_, err = io.ReadFull(ws.bufrd, payload)
		if err != nil {
			return nil, err
		}

		if masked {
			for i := 0; i < len(payload); i++ {
				payload[i] ^= mask[i%4]
			}
		}

		// Handle close frame
		if opcode == 8 {
			return nil, io.EOF
		}

		// Handle control frames
		if opcode == 9 { // Ping
			// Send Pong
			_ = ws.WriteFrame(10, payload)
			continue
		}
		if opcode == 10 { // Pong
			continue
		}

		// We only process binary messages (2)
		if opcode == 2 {
			if !fin {
				// Fragmented frame handling tag
				return payload, nil
			}
			return payload, nil
		}
	}
}

func (ws *wsConn) WriteFrame(opcode byte, data []byte) error {
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()

	var header []byte
	header = append(header, 0x80|opcode) // FIN set + opcode

	length := len(data)
	if length < 126 {
		header = append(header, byte(length))
	} else if length <= 65535 {
		header = append(header, 126)
		extLen := make([]byte, 2)
		binary.BigEndian.PutUint16(extLen, uint16(length))
		header = append(header, extLen...)
	} else {
		header = append(header, 127)
		extLen := make([]byte, 8)
		binary.BigEndian.PutUint64(extLen, uint64(length))
		header = append(header, extLen...)
	}

	_, err := ws.bufwr.Write(header)
	if err != nil {
		return err
	}

	_, err = ws.bufwr.Write(data)
	if err != nil {
		return err
	}

	return ws.bufwr.Flush()
}

type wsConnStreamReader struct {
	ws     *wsConn
	reader io.Reader
}

func (r *wsConnStreamReader) Read(p []byte) (int, error) {
	for {
		if r.reader != nil {
			n, err := r.reader.Read(p)
			if err == io.EOF {
				r.reader = nil
				continue
			}
			return n, err
		}
		payload, err := r.ws.NextMessage()
		if err != nil {
			return 0, err
		}
		r.reader = bytes.NewReader(payload)
	}
}

type wsConnStreamWriter struct {
	ws *wsConn
}

func (w *wsConnStreamWriter) Write(p []byte) (int, error) {
	err := w.ws.WriteFrame(2, p) // opcode 2 represents Binary message
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
