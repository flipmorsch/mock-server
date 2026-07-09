package main

import "testing"

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1":    true,
		"127.0.0.53":   true,
		"::1":          true,
		"localhost":    true,
		"0.0.0.0":      false,
		"":             false, // bind-all — the exposure case worth warning about
		"192.168.1.10": false,
	}
	for host, want := range cases {
		if got := isLoopback(host); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}
