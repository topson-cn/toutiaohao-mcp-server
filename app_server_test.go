package main

import "testing"

func TestServerListenAddrIsLoopback(t *testing.T) {
	if got := serverListenAddr("8080"); got != "127.0.0.1:8080" {
		t.Fatalf("serverListenAddr() = %q, want %q", got, "127.0.0.1:8080")
	}
}
