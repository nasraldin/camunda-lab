package ui

import "testing"

func TestAssertLoopback(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "::1"} {
		if err := assertLoopback(host); err != nil {
			t.Fatalf("%s: %v", host, err)
		}
	}
	if err := assertLoopback("0.0.0.0"); err == nil {
		t.Fatal("expected error for 0.0.0.0")
	}
	if err := assertLoopback("192.168.1.10"); err == nil {
		t.Fatal("expected error for LAN IP")
	}
}
