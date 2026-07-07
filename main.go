package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const version = "0.2.0"

func main() {
	listenOverride := flag.String("listen", "", "override listen address (e.g., 127.0.0.1:8080)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mock-server [--listen addr:port] <config.yaml>\n")
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
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()

	for i := range h.config.Rules {
		rule := &h.config.Rules[i]
		if match(rule, r, body) {
			if rule.Response.delayDuration > 0 {
				time.Sleep(rule.Response.delayDuration)
			}
			log.Printf("%s %s → %d (matched: %s)", r.Method, r.URL.Path, rule.Response.Status, rule.Name)
			writeResponse(w, &rule.Response, r, body)
			return
		}
	}

	log.Printf("%s %s → 404 (no match)", r.Method, r.URL.Path)
	http.NotFound(w, r)
}

func writeResponse(w http.ResponseWriter, resp *Response, r *http.Request, reqBody []byte) {
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
		body, err = executeTemplate(resp.Body, r, reqBody)
		if err != nil {
			log.Printf("template error: %v", err)
			return
		}
	}
	fmt.Fprint(w, body)
}
