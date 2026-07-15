package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

const (
	opcodeContinuation = 0x0
	opcodeText         = 0x1
	opcodeBinary       = 0x2
	opcodeClose        = 0x8
	opcodePing         = 0x9
	opcodePong         = 0xA
)

var (
	errProtocol      = errors.New("websocket protocol error")
	errMessageTooBig = errors.New("websocket message too large")
)

type wsConn struct {
	conn            net.Conn
	bufrd           *bufio.Reader
	bufwr           *bufio.Writer
	maxMessageBytes int64
	writeMu         sync.Mutex
}

type wsFrame struct {
	fin     bool
	opcode  byte
	payload []byte
}

func (ws *wsConn) NextMessage() ([]byte, error) {
	var message []byte
	var fragmented bool

	for {
		frame, err := ws.readFrame()
		if err != nil {
			return nil, err
		}

		switch frame.opcode {
		case opcodeClose:
			_ = ws.WriteFrame(opcodeClose, frame.payload)
			return nil, io.EOF
		case opcodePing:
			if err := ws.WriteFrame(opcodePong, frame.payload); err != nil {
				return nil, err
			}
			continue
		case opcodePong:
			continue
		case opcodeText:
			return nil, fmt.Errorf("%w: text frames are unsupported", errProtocol)
		case opcodeBinary:
			if fragmented {
				return nil, fmt.Errorf("%w: new data frame during fragmented message", errProtocol)
			}
			if frame.fin {
				return frame.payload, nil
			}
			fragmented = true
			if int64(len(message))+int64(len(frame.payload)) > ws.maxMessageBytes {
				return nil, errMessageTooBig
			}
			message = append(message, frame.payload...)
		case opcodeContinuation:
			if !fragmented {
				return nil, fmt.Errorf("%w: unexpected continuation frame", errProtocol)
			}
			if int64(len(message))+int64(len(frame.payload)) > ws.maxMessageBytes {
				return nil, errMessageTooBig
			}
			message = append(message, frame.payload...)
			if frame.fin {
				return message, nil
			}
		default:
			return nil, fmt.Errorf("%w: unsupported opcode", errProtocol)
		}
	}
}

func (ws *wsConn) readFrame() (wsFrame, error) {
	var header [2]byte
	if _, err := io.ReadFull(ws.bufrd, header[:]); err != nil {
		return wsFrame{}, err
	}
	if header[0]&0x70 != 0 {
		return wsFrame{}, fmt.Errorf("%w: RSV bits must be zero", errProtocol)
	}

	frame := wsFrame{
		fin:    header[0]&0x80 != 0,
		opcode: header[0] & 0x0F,
	}
	if header[1]&0x80 == 0 {
		return wsFrame{}, fmt.Errorf("%w: client frames must be masked", errProtocol)
	}

	payloadLength := uint64(header[1] & 0x7F)
	switch payloadLength {
	case 126:
		var extended [2]byte
		if _, err := io.ReadFull(ws.bufrd, extended[:]); err != nil {
			return wsFrame{}, err
		}
		payloadLength = uint64(binary.BigEndian.Uint16(extended[:]))
		if payloadLength < 126 {
			return wsFrame{}, fmt.Errorf("%w: non-minimal payload length", errProtocol)
		}
	case 127:
		var extended [8]byte
		if _, err := io.ReadFull(ws.bufrd, extended[:]); err != nil {
			return wsFrame{}, err
		}
		if extended[0]&0x80 != 0 {
			return wsFrame{}, fmt.Errorf("%w: invalid 64-bit payload length", errProtocol)
		}
		payloadLength = binary.BigEndian.Uint64(extended[:])
		if payloadLength < 65536 {
			return wsFrame{}, fmt.Errorf("%w: non-minimal payload length", errProtocol)
		}
	}

	isControl := frame.opcode&0x08 != 0
	if isControl && (!frame.fin || payloadLength > 125) {
		return wsFrame{}, fmt.Errorf("%w: invalid control frame", errProtocol)
	}
	if payloadLength > uint64(ws.maxMessageBytes) {
		return wsFrame{}, errMessageTooBig
	}

	var mask [4]byte
	if _, err := io.ReadFull(ws.bufrd, mask[:]); err != nil {
		return wsFrame{}, err
	}

	frame.payload = make([]byte, int(payloadLength))
	if _, err := io.ReadFull(ws.bufrd, frame.payload); err != nil {
		return wsFrame{}, err
	}
	for i := range frame.payload {
		frame.payload[i] ^= mask[i%4]
	}
	return frame, nil
}

func (ws *wsConn) WriteFrame(opcode byte, payload []byte) error {
	if len(payload) > int(ws.maxMessageBytes) {
		return errMessageTooBig
	}
	if opcode&0x08 != 0 && len(payload) > 125 {
		return fmt.Errorf("%w: control payload is too large", errProtocol)
	}

	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()

	header := make([]byte, 0, 10)
	header = append(header, 0x80|opcode)

	switch length := len(payload); {
	case length < 126:
		header = append(header, byte(length))
	case length <= 65535:
		header = append(header, 126, byte(length>>8), byte(length))
	default:
		header = append(header, 127)
		var extended [8]byte
		binary.BigEndian.PutUint64(extended[:], uint64(length))
		header = append(header, extended[:]...)
	}

	if _, err := ws.bufwr.Write(header); err != nil {
		return err
	}
	if _, err := ws.bufwr.Write(payload); err != nil {
		return err
	}
	return ws.bufwr.Flush()
}

type wsStreamReader struct {
	ws     *wsConn
	reader io.Reader
}

func (r *wsStreamReader) Read(buffer []byte) (int, error) {
	for {
		if r.reader != nil {
			n, err := r.reader.Read(buffer)
			if !errors.Is(err, io.EOF) {
				return n, err
			}
			r.reader = nil
			if n > 0 {
				return n, nil
			}
		}

		payload, err := r.ws.NextMessage()
		if err != nil {
			return 0, err
		}
		r.reader = bytes.NewReader(payload)
	}
}

type wsStreamWriter struct {
	ws *wsConn
}

func (w *wsStreamWriter) Write(payload []byte) (int, error) {
	if err := w.ws.WriteFrame(opcodeBinary, payload); err != nil {
		return 0, err
	}
	return len(payload), nil
}
