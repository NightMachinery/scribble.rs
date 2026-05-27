package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/cors"
	"github.com/scribble-rs/scribble.rs/internal/api"
	"github.com/scribble-rs/scribble.rs/internal/config"
	"github.com/scribble-rs/scribble.rs/internal/frontend"
	"github.com/scribble-rs/scribble.rs/internal/game"
	"github.com/scribble-rs/scribble.rs/internal/identity"
	"github.com/scribble-rs/scribble.rs/internal/state"
	"github.com/scribble-rs/scribble.rs/internal/version"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalln("error loading configuration:", err)
	}

	if err := identity.Init(cfg.PlayerIdentityStorePath); err != nil {
		log.Fatalln("error loading player identity store:", err)
	}

	game.SetDisconnectGrace(cfg.PlayerReconnectGrace)

	log.Printf("Starting Scribble.rs version '%s'\n", version.Version)

	if cfg.CPUProfilePath != "" {
		log.Println("Starting CPU profiling ....")
		cpuProfileFile, err := os.Create(cfg.CPUProfilePath)
		if err != nil {
			log.Fatal("error creating cpuprofile file:", err)
		}
		if err := pprof.StartCPUProfile(cpuProfileFile); err != nil {
			log.Fatal("error starting cpu profiling:", err)
		}
	}

	router := http.NewServeMux()
	corsWrapper := cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORS.AllowedOrigins,
		AllowCredentials: cfg.CORS.AllowCredentials,
	})
	register := func(method, path string, handler http.HandlerFunc) {
		// Each path needs to start with a slash anyway, so this is convenient.
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		log.Printf("Registering route: %s %s\n", method, path)
		router.HandleFunc(fmt.Sprintf("%s %s", method, path), corsWrapper(handler).ServeHTTP)
	}

	// Healthcheck for deployments with monitoring if required.
	register("GET", path.Join(cfg.RootPath, "health"), func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})

	api.NewHandler(cfg).SetupRoutes(cfg.RootPath, register)

	frontendHandler, err := frontend.NewHandler(cfg)
	if err != nil {
		log.Fatal("error setting up frontend:", err)
	}
	frontendHandler.SetupRoutes(register)

	if cfg.LobbyCleanup.Interval > 0 {
		state.LaunchCleanupRoutine(cfg.LobbyCleanup)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		defer os.Exit(0)

		log.Printf("Received %s, gracefully shutting down.\n", <-signalChan)

		state.ShutdownLobbiesGracefully()
		if cfg.CPUProfilePath != "" {
			pprof.StopCPUProfile()
			log.Println("Finished CPU profiling.")
		}
	}()

	address := fmt.Sprintf("%s:%d", cfg.NetworkAddress, cfg.Port)
	log.Println("Started, listening on: http://" + address)

	httpServer := &http.Server{
		Addr: address,
		Handler: redirectHTTPToHTTPS(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" && r.URL.Path[len(r.URL.Path)-1] == '/' {
				r.URL.Path = r.URL.Path[:len(r.URL.Path)-1]
			}

			router.ServeHTTP(w, r)
		})),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatalln(httpServer.ListenAndServe())
}

func redirectHTTPToHTTPS(cfg *config.Config, next http.Handler) http.Handler {
	if cfg.RootURL == "" {
		return next
	}

	rootURL, err := url.Parse(cfg.RootURL)
	if err != nil || rootURL.Scheme != "https" || rootURL.Host == "" {
		return next
	}

	healthPath := "/" + path.Join(cfg.RootPath, "health")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestScheme(r) == "https" || r.URL.Path == healthPath {
			next.ServeHTTP(w, r)
			return
		}

		redirectURL := *r.URL
		redirectURL.Scheme = "https"
		redirectURL.Host = hostForRedirect(r, rootURL.Host)
		http.Redirect(w, r, redirectURL.String(), http.StatusPermanentRedirect)
	})
}

func requestScheme(request *http.Request) string {
	if request.TLS != nil {
		return "https"
	}

	if scheme := firstForwardedHeaderValue(request.Header.Get("X-Forwarded-Proto")); scheme != "" {
		return strings.ToLower(scheme)
	}

	if forwarded := firstForwardedHeaderValue(request.Header.Get("Forwarded")); forwarded != "" {
		for part := range strings.SplitSeq(forwarded, ";") {
			key, value, found := strings.Cut(strings.TrimSpace(part), "=")
			if !found || !strings.EqualFold(key, "proto") {
				continue
			}
			return strings.ToLower(strings.Trim(value, "`'\""))
		}
	}

	return "http"
}

func firstForwardedHeaderValue(value string) string {
	if value == "" {
		return ""
	}

	firstValue := strings.TrimSpace(strings.Split(value, ",")[0])
	return strings.Trim(firstValue, "`'\"")
}

func hostForRedirect(request *http.Request, fallback string) string {
	host := request.Host
	if forwardedHost := firstForwardedHeaderValue(request.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	if host == "" {
		host = fallback
	}
	return host
}
