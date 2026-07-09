package mock_test

import (
	"net/http"
	"testing"

	"github.com/flipmorsch/mock-server/mock"
)

// fakeTB records StartT's interactions without aborting the goroutine the way a
// real *testing.T.Fatalf would, so both the success and error paths are testable.
type fakeTB struct {
	fatal    bool
	cleanups []func()
}

func (f *fakeTB) Helper()               {}
func (f *fakeTB) Fatalf(string, ...any) { f.fatal = true }
func (f *fakeTB) Cleanup(fn func())     { f.cleanups = append(f.cleanups, fn) }

func TestStartT(t *testing.T) {
	f := &fakeTB{}
	m := mock.StartT(f, `
rules:
  - request: {method: GET, path: /ok}
    response: {status: 200, body: ok}
`)
	if f.fatal {
		t.Fatal("StartT fataled on a valid config")
	}
	if m == nil {
		t.Fatal("StartT returned nil for a valid config")
	}
	if len(f.cleanups) != 1 {
		t.Fatalf("expected 1 registered cleanup, got %d", len(f.cleanups))
	}

	resp, err := http.Get(m.URL() + "/ok")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	f.cleanups[0]() // simulate t.Cleanup firing
	if _, err := http.Get(m.URL() + "/ok"); err == nil {
		t.Error("expected request to fail after cleanup closed the server")
	}
}

func TestStartTFatalsOnBadConfig(t *testing.T) {
	f := &fakeTB{}
	m := mock.StartT(f, "not: [valid")
	if !f.fatal {
		t.Error("StartT should fatal on an invalid config")
	}
	if m != nil {
		t.Error("StartT should return nil after fatal")
	}
	if len(f.cleanups) != 0 {
		t.Error("StartT should not register cleanup after fatal")
	}
}
