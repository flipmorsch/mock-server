package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const version = "0.3.0"

func main() {
	listenOverride := flag.String("listen", "", "override listen address (e.g., 127.0.0.1:8080)")
	showVersion := flag.Bool("version", false, "print version and exit")
	uiEnabled := flag.Bool("ui", false, "enable embedded Web UI at /_ui/")
	fixturesDir := flag.String("fixtures-dir", "", "directory for uploaded fixture files (default: ./fixtures)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mock-server [flags] <config.yaml>\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("mock-server version", version)
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	configPath := flag.Arg(0)

	var cfg *Config
	var err error
	if *uiEnabled {
		cfg, err = loadOrEmpty(configPath)
	} else {
		cfg, err = LoadConfig(configPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fixDir := *fixturesDir
	if fixDir == "" {
		fixDir = "./fixtures"
	}

	srv := newServer(cfg, configPath, &Journal{}, *uiEnabled, fixDir)

	handler := newHandler(srv)

	addr := srv.ListenAddr()
	if *listenOverride != "" {
		addr = *listenOverride
	}

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func loadOrEmpty(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Config{}, nil
	}
	return LoadConfig(path)
}

type handler struct {
	srv *Server
}

func newHandler(srv *Server) http.Handler {
	return &handler{srv: srv}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/_ui/") {
		if !h.srv.uiEnabled {
			http.NotFound(w, r)
			return
		}
		uiHandler(h.srv)(w, r)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/__admin/") {
		adminHandler(h.srv.journal)(w, r)
		return
	}

	body, _ := io.ReadAll(r.Body)
	r.Body.Close()

	reqHeaders := make(map[string]string)
	for k := range r.Header {
		reqHeaders[http.CanonicalHeaderKey(k)] = r.Header.Get(k)
	}

	rules := h.srv.Rules()
	for i := range rules {
		rule := &rules[i]
		if match(rule, r, body) {
			if rule.Response.delayDuration > 0 {
				time.Sleep(rule.Response.delayDuration)
			}
			log.Printf("%s %s → %d (matched: %s)", r.Method, r.URL.Path, rule.Response.Status, rule.Name)
			h.srv.journal.Record(r.Method, r.URL.Path, r.URL.RawQuery, reqHeaders, body, rule.Name, rule.Response.Status)
			writeResponse(w, &rule.Response, r, body, h.srv.journal)
			return
		}
	}

	log.Printf("%s %s → 404 (no match)", r.Method, r.URL.Path)
	h.srv.journal.Record(r.Method, r.URL.Path, r.URL.RawQuery, reqHeaders, body, "", 404)
	http.NotFound(w, r)
}

func writeResponse(w http.ResponseWriter, resp *Response, r *http.Request, reqBody []byte, journal *Journal) {
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	if _, ok := resp.Headers["Content-Type"]; !ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.WriteHeader(resp.Status)
	if resp.Body == "" {
		return
	}
	body := resp.Body
	if resp.Template {
		var err error
		body, err = executeTemplate(resp.Body, r, reqBody, journal)
		if err != nil {
			log.Printf("template error: %v", err)
			return
		}
	}
	fmt.Fprint(w, body)
}
