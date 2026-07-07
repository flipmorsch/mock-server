package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	listenOverride := flag.String("listen", "", "override listen address (e.g., 127.0.0.1:8080)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mock-server [--listen addr:port] <config.yaml>\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	configPath := flag.Arg(0)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	addr := cfg.listenAddr()
	if *listenOverride != "" {
		addr = *listenOverride
	}

	handler := newHandler(cfg)

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

type handler struct {
	config *Config
}

func newHandler(cfg *Config) http.Handler {
	return &handler{config: cfg}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for i := range h.config.Rules {
		rule := &h.config.Rules[i]
		if match(rule, r) {
			log.Printf("%s %s → %d (matched: %s)", r.Method, r.URL.Path, rule.Response.Status, rule.Name)
			writeResponse(w, &rule.Response)
			return
		}
	}

	log.Printf("%s %s → 404 (no match)", r.Method, r.URL.Path)
	http.NotFound(w, r)
}

func writeResponse(w http.ResponseWriter, resp *Response) {
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	if _, ok := resp.Headers["Content-Type"]; !ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.WriteHeader(resp.Status)
	if resp.Body != "" {
		fmt.Fprint(w, resp.Body)
	}
}
