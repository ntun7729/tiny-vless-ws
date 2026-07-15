package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestWSNextMessageFragmentedWithPing(t *testing.T) {
	t.Parallel()

	input := bytes.Join([][]byte{
		maskedFrame(opcodeBinary, false, []byte("hel")),
		maskedFrame(opcodePing, true, []byte("x")),
		maskedFrame(opcodeContinuation, true, []byte("lo")),
	}, nil)
	output := &bytes.Buffer{}

	ws := &wsConn{
		bufrd:           bufio.NewReader(bytes.NewReader(input)),
		bufwr:           bufio.NewWriter(output),
		maxMessageBytes: 1024,
	}

	message, err := ws.NextMessage()
	if err != nil {
		t.Fatalf("NextMessage: %v", err)
	}
	if string(message) != "hello" {
		t.Fatalf("message = %q", message)
	}

	if got, want := output.Bytes(), []byte{0x80 | opcodePong, 1, 'x'}; !bytes.Equal(got, want) {
		t.Fatalf("pong = %v, want %v", got, want)
	}
}

func TestWSRejectsUnmaskedAndOversizedFrames(t *testing.T) {
	t.Parallel()

	t.Run("unmasked", func(t *testing.T) {
		ws := &wsConn{
			bufrd:           bufio.NewReader(bytes.NewReader([]byte{0x82, 0x01, 'x'})),
			bufwr:           bufio.NewWriter(io.Discard),
			maxMessageBytes: 1024,
		}
		if _, err := ws.NextMessage(); !errors.Is(err, errProtocol) {
			t.Fatalf("error = %v, want protocol error", err)
		}
	})

	t.Run("oversized", func(t *testing.T) {
		frame := []byte{0x82, 0xFE, 0x08, 0x00}
		ws := &wsConn{
			bufrd:           bufio.NewReader(bytes.NewReader(frame)),
			bufwr:           bufio.NewWriter(io.Discard),
			maxMessageBytes: 1024,
		}
		if _, err := ws.NextMessage(); !errors.Is(err, errMessageTooBig) {
			t.Fatalf("error = %v, want message-too-big", err)
		}
	})
}

func maskedFrame(opcode byte, fin bool, payload []byte) []byte {
	first := opcode
	if fin {
		first |= 0x80
	}

	frame := []byte{first}
	switch length := len(payload); {
	case length < 126:
		frame = append(frame, 0x80|byte(length))
	case length <= 65535:
		frame = append(frame, 0x80|126, byte(length>>8), byte(length))
	default:
		frame = append(frame, 0x80|127)
		var extended [8]byte
		binary.BigEndian.PutUint64(extended[:], uint64(length))
		frame = append(frame, extended[:]...)
	}

	mask := [4]byte{1, 2, 3, 4}
	frame = append(frame, mask[:]...)
	for i, value := range payload {
		frame = append(frame, value^mask[i%4])
	}
	return frame
}
