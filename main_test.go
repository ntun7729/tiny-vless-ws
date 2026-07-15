package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestParseUUID(t *testing.T) {
	const canonical = "00112233-4455-6677-8899-aabbccddeeff"
	want := [16]byte{
		0x00, 0x11, 0x22, 0x33,
		0x44, 0x55,
		0x66, 0x77,
		0x88, 0x99,
		0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
	}

	for _, input := range []string{
		canonical,
		"00112233445566778899aabbccddeeff",
		"00112233-4455-6677-8899-AABBCCDDEEFF",
	} {
		t.Run(input, func(t *testing.T) {
			got, err := parseUUID(input)
			if err != nil {
				t.Fatalf("parseUUID(%q) returned error: %v", input, err)
			}
			if got != want {
				t.Fatalf("parseUUID(%q) = %x, want %x", input, got, want)
			}
		})
	}

	for _, input := range []string{
		"",
		"00112233-4455-6677-8899-aabbccddee",
		"00112233-4455-6677-8899-aabbccddeefg",
	} {
		t.Run("invalid_"+input, func(t *testing.T) {
			if _, err := parseUUID(input); err == nil {
				t.Fatalf("parseUUID(%q) unexpectedly succeeded", input)
			}
		})
	}
}

func TestWSConnNextMessageUnmasksBinaryFrame(t *testing.T) {
	frame := makeClientFrame(true, 2, []byte("hello"))
	var written bytes.Buffer
	ws := &wsConn{
		bufrd: bufio.NewReader(bytes.NewReader(frame)),
		bufwr: bufio.NewWriter(&written),
	}

	got, err := ws.NextMessage()
	if err != nil {
		t.Fatalf("NextMessage() returned error: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("NextMessage() = %q, want %q", got, "hello")
	}
}

func TestWSConnNextMessageReassemblesFragments(t *testing.T) {
	var input bytes.Buffer
	input.Write(makeClientFrame(false, 2, []byte("hello ")))
	input.Write(makeClientFrame(true, 0, []byte("world")))

	ws := &wsConn{
		bufrd: bufio.NewReader(&input),
		bufwr: bufio.NewWriter(io.Discard),
	}

	got, err := ws.NextMessage()
	if err != nil {
		t.Fatalf("NextMessage() returned error: %v", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("NextMessage() = %q, want %q", got, "hello world")
	}
}

func TestWSConnNextMessageRepliesToPing(t *testing.T) {
	var input bytes.Buffer
	input.Write(makeClientFrame(true, 9, []byte("ok")))
	input.Write(makeClientFrame(true, 2, []byte("data")))

	var output bytes.Buffer
	ws := &wsConn{
		bufrd: bufio.NewReader(&input),
		bufwr: bufio.NewWriter(&output),
	}

	got, err := ws.NextMessage()
	if err != nil {
		t.Fatalf("NextMessage() returned error: %v", err)
	}
	if string(got) != "data" {
		t.Fatalf("NextMessage() = %q, want %q", got, "data")
	}

	wantPong := []byte{0x8a, 0x02, 'o', 'k'}
	if !bytes.Equal(output.Bytes(), wantPong) {
		t.Fatalf("pong frame = %x, want %x", output.Bytes(), wantPong)
	}
}

func TestWSConnWriteFrameLengthEncoding(t *testing.T) {
	tests := []struct {
		name       string
		size       int
		lengthCode byte
		headerSize int
	}{
		{name: "small", size: 125, lengthCode: 125, headerSize: 2},
		{name: "medium", size: 126, lengthCode: 126, headerSize: 4},
		{name: "large", size: 65536, lengthCode: 127, headerSize: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := bytes.Repeat([]byte{0x5a}, tt.size)
			var output bytes.Buffer
			ws := &wsConn{bufwr: bufio.NewWriter(&output)}

			if err := ws.WriteFrame(2, payload); err != nil {
				t.Fatalf("WriteFrame() returned error: %v", err)
			}

			frame := output.Bytes()
			if len(frame) != tt.headerSize+tt.size {
				t.Fatalf("frame length = %d, want %d", len(frame), tt.headerSize+tt.size)
			}
			if frame[0] != 0x82 {
				t.Fatalf("first header byte = 0x%x, want 0x82", frame[0])
			}
			if frame[1] != tt.lengthCode {
				t.Fatalf("length code = %d, want %d", frame[1], tt.lengthCode)
			}

			switch tt.lengthCode {
			case 126:
				if got := int(binary.BigEndian.Uint16(frame[2:4])); got != tt.size {
					t.Fatalf("encoded length = %d, want %d", got, tt.size)
				}
			case 127:
				if got := int(binary.BigEndian.Uint64(frame[2:10])); got != tt.size {
					t.Fatalf("encoded length = %d, want %d", got, tt.size)
				}
			}

			if !bytes.Equal(frame[tt.headerSize:], payload) {
				t.Fatal("payload changed while writing frame")
			}
		})
	}
}

func makeClientFrame(fin bool, opcode byte, payload []byte) []byte {
	const maskBit = byte(0x80)
	mask := [4]byte{0x11, 0x22, 0x33, 0x44}

	first := opcode
	if fin {
		first |= 0x80
	}

	frame := []byte{first}
	switch {
	case len(payload) < 126:
		frame = append(frame, maskBit|byte(len(payload)))
	case len(payload) <= 65535:
		frame = append(frame, maskBit|126, byte(len(payload)>>8), byte(len(payload)))
	default:
		frame = append(frame, maskBit|127)
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(payload)))
		frame = append(frame, length[:]...)
	}

	frame = append(frame, mask[:]...)
	for i, b := range payload {
		frame = append(frame, b^mask[i%len(mask)])
	}
	return frame
}
