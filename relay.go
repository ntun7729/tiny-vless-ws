package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
)

const tcpRelayBufferSize = 32 * 1024

func (s *proxyServer) handleTCP(ws *wsConn, request vlessRequest) {
	target, err := s.dialer.Dial("tcp", destination(request.address, request.port))
	if err != nil {
		return
	}
	defer target.Close()

	if err := ws.WriteFrame(opcodeBinary, []byte{0, 0}); err != nil {
		return
	}

	reader := &wsStreamReader{ws: ws, reader: bytes.NewReader(request.payload)}
	writer := &wsStreamWriter{ws: ws}
	relayBidirectional(ws.conn, target,
		func() { _, _ = io.Copy(target, reader) },
		func() { _, _ = copyTCPToWebSocket(writer, target, ws.maxMessageBytes) },
	)
}

func copyTCPToWebSocket(writer io.Writer, target io.Reader, maxMessageBytes int64) (int64, error) {
	if maxMessageBytes <= 0 {
		return 0, errMessageTooBig
	}

	bufferSize := tcpRelayBufferSize
	if int64(bufferSize) > maxMessageBytes {
		bufferSize = int(maxMessageBytes)
	}

	return io.CopyBuffer(writer, target, make([]byte, bufferSize))
}

func (s *proxyServer) handleUDP(ws *wsConn, request vlessRequest) {
	target, err := s.dialer.Dial("udp", destination(request.address, request.port))
	if err != nil {
		return
	}
	defer target.Close()

	if err := ws.WriteFrame(opcodeBinary, []byte{0, 0}); err != nil {
		return
	}

	reader := &wsStreamReader{ws: ws, reader: bytes.NewReader(request.payload)}
	writer := &wsStreamWriter{ws: ws}

	relayBidirectional(ws.conn, target,
		func() { relayUDPToTarget(reader, target) },
		func() { relayUDPToWebSocket(target, writer) },
	)
}

func relayBidirectional(client, target net.Conn, clientToTarget, targetToClient func()) {
	done := make(chan struct{}, 2)

	go func() {
		clientToTarget()
		done <- struct{}{}
	}()
	go func() {
		targetToClient()
		done <- struct{}{}
	}()

	<-done
	_ = target.Close()
	_ = client.Close()
	<-done
}

func relayUDPToTarget(reader io.Reader, target net.Conn) {
	lengthBuffer := make([]byte, 2)
	packet := make([]byte, 65535)

	for {
		if _, err := io.ReadFull(reader, lengthBuffer); err != nil {
			return
		}
		packetLength := int(binary.BigEndian.Uint16(lengthBuffer))
		if packetLength == 0 {
			return
		}
		if _, err := io.ReadFull(reader, packet[:packetLength]); err != nil {
			return
		}
		if _, err := target.Write(packet[:packetLength]); err != nil {
			return
		}
	}
}

func relayUDPToWebSocket(target net.Conn, writer io.Writer) {
	packet := make([]byte, 65535)
	framed := make([]byte, 2+len(packet))

	for {
		n, err := target.Read(packet)
		if err != nil {
			return
		}
		if n == 0 {
			continue
		}
		binary.BigEndian.PutUint16(framed[:2], uint16(n))
		copy(framed[2:2+n], packet[:n])
		if _, err := writer.Write(framed[:2+n]); err != nil {
			return
		}
	}
}
