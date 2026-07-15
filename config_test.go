package main

import (
	"encoding/binary"
	"testing"
)

func TestParseUUID(t *testing.T) {
	t.Parallel()

	valid, err := parseUUID("00112233-4455-6677-8899-aabbccddeeff")
	if err != nil {
		t.Fatalf("parseUUID(valid): %v", err)
	}
	if got := binary.BigEndian.Uint32(valid[:4]); got != 0x00112233 {
		t.Fatalf("unexpected UUID prefix: %08x", got)
	}

	for _, value := range []string{"", "not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		value := value
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if _, err := parseUUID(value); err == nil {
				t.Fatalf("parseUUID(%q) unexpectedly succeeded", value)
			}
		})
	}
}

func TestConfigParsers(t *testing.T) {
	t.Parallel()

	if port, err := parsePort(""); err != nil || port != defaultPort {
		t.Fatalf("parsePort default = %d, %v", port, err)
	}
	if _, err := parsePort("0"); err == nil {
		t.Fatal("parsePort accepted zero")
	}
	if path, err := parseWSPath("vless"); err != nil || path != "/vless" {
		t.Fatalf("parseWSPath = %q, %v", path, err)
	}
	for _, value := range []string{"/healthz", "/a/../b", "/a?token=x"} {
		if _, err := parseWSPath(value); err == nil {
			t.Fatalf("parseWSPath(%q) unexpectedly succeeded", value)
		}
	}
}
