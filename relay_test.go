package main

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type plainReader struct {
	reader io.Reader
}

func (r plainReader) Read(buffer []byte) (int, error) {
	return r.reader.Read(buffer)
}

type cappedWriter struct {
	maxWrite int
	total    int
	writes   []int
}

func (w *cappedWriter) Write(payload []byte) (int, error) {
	if len(payload) > w.maxWrite {
		return 0, errMessageTooBig
	}
	w.total += len(payload)
	w.writes = append(w.writes, len(payload))
	return len(payload), nil
}

func TestCopyTCPToWebSocketHonorsMaxMessageSize(t *testing.T) {
	t.Parallel()

	const limit = 1024
	payload := bytes.Repeat([]byte{'x'}, 3*limit+17)
	writer := &cappedWriter{maxWrite: limit}
	reader := plainReader{reader: bytes.NewReader(payload)}

	copied, err := copyTCPToWebSocket(writer, reader, limit)
	if err != nil {
		t.Fatalf("copyTCPToWebSocket: %v", err)
	}
	if copied != int64(len(payload)) {
		t.Fatalf("copied = %d, want %d", copied, len(payload))
	}
	if writer.total != len(payload) {
		t.Fatalf("writer total = %d, want %d", writer.total, len(payload))
	}
	if len(writer.writes) < 2 {
		t.Fatalf("writes = %v, want multiple bounded writes", writer.writes)
	}
	for _, size := range writer.writes {
		if size > limit {
			t.Fatalf("write size = %d, exceeds limit %d", size, limit)
		}
	}
}

func TestCopyTCPToWebSocketRejectsNonPositiveLimit(t *testing.T) {
	t.Parallel()

	_, err := copyTCPToWebSocket(io.Discard, plainReader{reader: bytes.NewReader(nil)}, 0)
	if !errors.Is(err, errMessageTooBig) {
		t.Fatalf("error = %v, want message-too-big", err)
	}
}
