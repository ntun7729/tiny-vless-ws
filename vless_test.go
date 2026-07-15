package main

import "testing"

func TestParseVLESSRequestDomain(t *testing.T) {
	t.Parallel()

	uuid, err := parseUUID("00112233-4455-6677-8899-aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}

	domain := "example.com"
	message := make([]byte, 0, 64)
	message = append(message, 0)
	message = append(message, uuid[:]...)
	message = append(message, 0)
	message = append(message, commandTCP)
	message = append(message, 0x01, 0xBB)
	message = append(message, 2, byte(len(domain)))
	message = append(message, domain...)
	message = append(message, "hello"...)

	request, err := parseVLESSRequest(message, uuid)
	if err != nil {
		t.Fatalf("parseVLESSRequest: %v", err)
	}
	if request.command != commandTCP || request.address != domain || request.port != 443 {
		t.Fatalf("unexpected request: %+v", request)
	}
	if string(request.payload) != "hello" {
		t.Fatalf("payload = %q", request.payload)
	}

	message[1] ^= 0xFF
	if _, err := parseVLESSRequest(message, uuid); err == nil {
		t.Fatal("invalid UUID unexpectedly accepted")
	}
}
