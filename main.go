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

	"mock-server/internal/rule"
	"mock-server/internal/server"
	"mock-server/internal/ui"
)

const version = "0.4.0"

func main() {
	listenOverride := flag.String("listen", "", "override listen address (e.g., 127.0.0.1:8080)")
	showVersion := flag.Bool("version", false, "print version and exit")
	uiEnabled := flag.Bool("ui", false, "enable embedded Web UI at /_ui/")
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

	var cfg *rule.Config
	var err error
	if *uiEnabled {
		cfg, err = rule.LoadOrEmpty(configPath)
	} else {
		cfg, err = rule.LoadConfig(configPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	journal := server.NewJournal()
	srv := server.NewServer(cfg, configPath, journal, *uiEnabled)

	addr := srv.ListenAddr()
	if *listenOverride != "" {
		addr = *listenOverride
	}

	h := &handler{srv: srv}

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

type handler struct {
	srv *server.Server
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/_ui/") {
		if !h.srv.UIEnabled() {
			http.NotFound(w, r)
			return
		}
		ui.Handler(h.srv, ui.StaticFS)(w, r)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/__admin/") {
		ui.AdminHandler(h.srv.Journal())(w, r)
		return
	}
	start := time.Now()
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()

	reqHeaders := make(map[string]string)

	for k := range r.Header {
		reqHeaders[http.CanonicalHeaderKey(k)] = r.Header.Get(k)
	}

	entry := server.JournalEntry{
		Method:  r.Method,
		Path:    r.URL.Path,
		Query:   r.URL.RawQuery,
		Headers: reqHeaders,
		Body:    string(body),
	}

	matched, misses := h.srv.MatchRule(r, body)
	entry.Explanations = misses
	if matched != nil {
		if matched.Response.DelayDuration > 0 {
			time.Sleep(matched.Response.DelayDuration)
		}
		entry.Duration = time.Since(start)
		log.Printf("%s %s → %d (matched: %s)", r.Method, r.URL.Path, matched.Response.Status, matched.Name)
		entry.Matched = matched.Name
		entry.MatchedID = matched.ID
		entry.Status = matched.Response.Status
		h.srv.Journal().Record(entry)
		writeResponse(w, &matched.Response, r, body, h.srv.Journal())
		return
	}

	entry.Duration = time.Since(start)
	log.Printf("%s %s → 404 (no match)", r.Method, r.URL.Path)
	entry.Status = 404
	h.srv.Journal().Record(entry)
	http.NotFound(w, r)
}

func writeResponse(w http.ResponseWriter, resp *rule.Response, r *http.Request, reqBody []byte, journal *server.Journal) {
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
		body, err = rule.ExecuteTemplate(resp.Body, r, reqBody, func(f *rule.RequestFilter) int64 { return int64(journal.Count(f)) })
		if err != nil {
			log.Printf("template error: %v", err)
			return
		}
	}
	fmt.Fprint(w, body)
}
