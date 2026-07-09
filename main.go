package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"mock-server/internal/rule"
	"mock-server/internal/server"
	"mock-server/internal/ui"
)

const version = "1.0.1"

func main() {
	listenOverride := flag.String("listen", "", "override listen address (e.g., 127.0.0.1:8080)")
	showVersion := flag.Bool("version", false, "print version and exit")
	uiEnabled := flag.Bool("ui", false, "enable embedded Web UI at /_ui/")
	tlsFlag := flag.Bool("tls", false, "serve HTTPS (self-signed cert if --tls-cert/--tls-key not given)")
	tlsCert := flag.String("tls-cert", "", "path to TLS certificate file (implies --tls)")
	tlsKey := flag.String("tls-key", "", "path to TLS private key file (implies --tls)")
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

	// --tls-cert and --tls-key are provided together, and either one implies TLS.
	if (*tlsCert == "") != (*tlsKey == "") {
		fmt.Fprintln(os.Stderr, "Error: --tls-cert and --tls-key must be provided together")
		os.Exit(1)
	}
	tlsEnabled := *tlsFlag || *tlsCert != ""
	srv.SetTLSEnabled(tlsEnabled)

	if host, _, err := net.SplitHostPort(addr); err == nil && !isLoopback(host) {
		log.Printf("warning: %s is not a loopback address — the request journal and /__admin/ API are unauthenticated and expose captured request data (sensitive headers redacted); prefer 127.0.0.1 or front it with an auth proxy", addr)
	}

	h := &handler{srv: srv}

	// Hot reload is headless-only (ADR-0004): under --ui the working copy owns
	// the rules, so SIGHUP is ignored there rather than reloading — and, since
	// unhandled SIGHUP would kill the process, ignoring it also protects the
	// unsaved working copy from an accidental `kill -HUP`.
	go watchReload(srv, *uiEnabled)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second, // Slowloris guard on the header read
	}

	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	log.Printf("listening on %s://%s", scheme, addr)

	// Single listener, HTTP xor HTTPS (ADR-0005). ListenAndServeTLS uses the
	// TLSConfig certificate when the file arguments are empty.
	var serveErr error
	switch {
	case *tlsCert != "":
		serveErr = httpServer.ListenAndServeTLS(*tlsCert, *tlsKey)
	case tlsEnabled:
		cert, err := server.GenerateSelfSigned()
		if err != nil {
			fmt.Fprintf(os.Stderr, "generating self-signed certificate: %v\n", err)
			os.Exit(1)
		}
		httpServer.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		log.Printf("self-signed certificate SHA-256: %s", server.CertFingerprint(cert))
		serveErr = httpServer.ListenAndServeTLS("", "")
	default:
		serveErr = httpServer.ListenAndServe()
	}
	if serveErr != nil && serveErr != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "server error: %v\n", serveErr)
		os.Exit(1)
	}
}

// isLoopback reports whether host is a loopback address (or empty/all-interfaces
// counts as non-loopback, since that's the exposure case worth warning about).
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// watchReload reloads the rule set from disk on SIGHUP. SIGHUP is never
// delivered on Windows, where reload is simply unavailable — restart instead.
func watchReload(srv *server.Server, uiEnabled bool) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP)
	for range sig {
		if uiEnabled {
			log.Print("SIGHUP ignored: hot reload is disabled under --ui")
			continue
		}
		n, err := srv.Reload()
		if err != nil {
			fmt.Fprintf(os.Stderr, "reload failed: %v; rules unchanged\n", err)
			continue
		}
		log.Printf("reloaded %d rules", n)
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
	body := resp.Body
	if resp.BodyFile != "" {
		data, err := os.ReadFile(resp.BodyFile)
		if err != nil {
			log.Printf("body_file error: %v", err)
			http.Error(w, "body_file read failed", http.StatusInternalServerError)
			return
		}
		body = string(data)
	}
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	if _, ok := resp.Headers["Content-Type"]; !ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.WriteHeader(resp.Status)
	if body == "" {
		return
	}
	if resp.Template {
		var err error
		body, err = rule.ExecuteTemplate(body, r, reqBody, func(f *rule.RequestFilter) int64 { return int64(journal.Count(f)) })
		if err != nil {
			log.Printf("template error: %v", err)
			return
		}
	}
	fmt.Fprint(w, body)
}
