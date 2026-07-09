package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sendProbe must switch to https and tolerate a self-signed cert when the
// server is serving TLS — otherwise the UI's real-probe test breaks under --tls.
func TestSendProbeOverTLS(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x" || r.Header.Get("X-Probe") != "1" {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(201)
		fmt.Fprint(w, "pong")
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "https://")
	res, err := sendProbe(addr, true, ProbeRequest{
		Method:  "GET",
		Path:    "/x",
		Headers: map[string]string{"X-Probe": "1"},
	})
	if err != nil {
		t.Fatalf("probe over TLS failed (scheme/skip-verify not wired?): %v", err)
	}
	if res.Status != 201 || res.Body != "pong" {
		t.Errorf("got %+v, want status 201 body pong", res)
	}
}
