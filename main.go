package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ares-sim/ares/internal/api"
	"github.com/ares-sim/ares/internal/scenario"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	var (
		addr        = flag.String("addr", envOr("ARES_ADDR", ":8080"), "listen address")
		dbPath      = flag.String("db", envOr("ARES_DB", "data/ares.db"), "sqlite database path")
		scenDir     = flag.String("scenarios", envOr("ARES_SCENARIOS", "scenarios"), "directory of external scenario json files")
		serveFE     = flag.Bool("frontend", envOr("ARES_FRONTEND", "") != "", "serve the built frontend from ./frontend/dist")
		frontendDir = flag.String("dist", envOr("ARES_DIST", "frontend/dist"), "built frontend directory")
	)
	flag.Parse()

	loader := scenario.NewLoader()
	if err := loader.OpenDB(*dbPath); err != nil {
		log.Printf("sqlite disabled: %v", err)
	}
	defer loader.Close()

	if err := loader.LoadEmbedded(); err != nil {
		log.Fatalf("embedded scenarios: %v", err)
	}
	if *scenDir != "" {
		if st, err := os.Stat(*scenDir); err == nil && st.IsDir() {
			if err := loader.LoadDir(*scenDir); err != nil {
				log.Printf("external scenarios skipped: %v", err)
			} else {
				log.Printf("loaded external scenarios from %s", *scenDir)
			}
		}
	}
	list := loader.List()
	log.Printf("loaded %d scenario(s)", len(list))
	for _, s := range list {
		log.Printf("  - %s (%s): %d nodes / %d connections", s.Name, s.ID, s.Nodes, s.Connections)
	}

	srv := api.NewServer(loader)
	mux := http.NewServeMux()
	srv.Routes(mux)

	if *serveFE {
		dist := *frontendDir
		if st, err := os.Stat(dist); err == nil && st.IsDir() {
			fs := http.FileServer(http.Dir(dist))
			mux.Handle("/", spaHandler(dist, fs))
			log.Printf("serving frontend from %s", dist)
		} else {
			log.Printf("frontend dist %q not found; API only", dist)
		}
	}

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           withCORS(withLogging(mux)),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("ARES listening on %s", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("shutting down")

	if e := srv.Engine(); e != nil {
		e.Shutdown()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

func spaHandler(dist string, fs http.Handler) http.Handler {
	fileServer := http.FileServer(http.Dir(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasPrefix(clean, "api/") || clean == "ws" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no such endpoint"}`))
			return
		}
		if clean == "" || strings.HasSuffix(clean, "/") {
			http.ServeFile(w, r, dist+"/index.html")
			return
		}
		if st, err := os.Stat(dist + "/" + clean); err == nil && !st.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, dist+"/index.html")
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.code == 0 {
		s.code = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	s.code = http.StatusSwitchingProtocols
	return h.Hijack()
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.code == 0 {
			rec.code = http.StatusOK
		}
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.code, time.Since(start).Round(time.Millisecond))
	})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
