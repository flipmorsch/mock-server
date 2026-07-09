package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/flipmorsch/mock-server/internal/rule"
	"github.com/flipmorsch/mock-server/internal/server"
	"github.com/flipmorsch/mock-server/internal/ui"
)

const version = "1.3.0"

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
	srv.SetLogger(log.Default())

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
		ui.AdminHandler(h.srv)(w, r)
		return
	}
	h.srv.ServeMock(w, r)
}
