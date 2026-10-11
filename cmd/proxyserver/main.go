package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sebmak/proxyserver/pkg/api"
	"github.com/sebmak/proxyserver/pkg/config"
	"github.com/sebmak/proxyserver/pkg/ddns"
	"github.com/sebmak/proxyserver/pkg/logger"
	"github.com/sebmak/proxyserver/pkg/proxy"
	"github.com/sebmak/proxyserver/pkg/store"
	"github.com/sebmak/proxyserver/pkg/tlsmanager"
	"github.com/sebmak/proxyserver/web"
	"golang.org/x/sync/errgroup"
)

func main() {
	logger.Info("Starting ProxyServer Gateway...")

	cfg := config.LoadConfig()

	// 1. Initialize persistent store
	st, err := store.NewStore(cfg.ConfigPath, cfg.LegacyConfig)
	if err != nil {
		logger.Error("Failed to initialize store: %v", err)
		os.Exit(1)
	}

	// 2. Initialize Health Checker & Proxy Engine
	healthChecker := proxy.NewHealthChecker(st)
	proxyEngine := proxy.NewEngine(st, healthChecker)

	// 3. Initialize Cloudflare DDNS Manager
	ddnsMgr := ddns.NewManager(st)

	// 4. Initialize TLS Certificate Manager
	tlsMgr, err := tlsmanager.NewManager(st, cfg.CertDir)
	if err != nil {
		logger.Error("Failed to initialize TLS manager: %v", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background routines
	healthChecker.Start(ctx)
	ddnsMgr.Start(ctx)

	// 5. Initialize Web UI & API Server
	apiServer := api.NewServer(cfg, st, proxyEngine, ddnsMgr)

	// Setup Web static files handler
	staticHandler := setupWebHandler()
	adminHandler := apiServer.Handler(staticHandler)

	adminHTTP := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.AdminPort),
		Handler:      adminHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// 6. Setup HTTP Reverse Proxy Server (also serves ACME challenges)
	proxyHTTP := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      tlsMgr.HTTPHandler(proxyEngine),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	// 7. Setup HTTPS Reverse Proxy Server
	proxyHTTPS := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPSPort),
		Handler:      proxyEngine,
		TLSConfig:    tlsMgr.GetTLSConfig(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	var g errgroup.Group

	// Admin Web UI Server
	g.Go(func() error {
		logger.Info("Management Web UI & API listening on http://0.0.0.0:%d", cfg.AdminPort)
		if err := adminHTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("admin server error: %w", err)
		}
		return nil
	})

	// HTTP Proxy Server
	g.Go(func() error {
		logger.Info("HTTP Reverse Proxy listening on :%d", cfg.HTTPPort)
		if err := proxyHTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http proxy error: %w", err)
		}
		return nil
	})

	// HTTPS Proxy Server
	g.Go(func() error {
		logger.Info("HTTPS Reverse Proxy listening on :%d", cfg.HTTPSPort)
		if err := proxyHTTPS.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("https proxy error: %w", err)
		}
		return nil
	})

	// Graceful shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Shutting down ProxyServer gracefully...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		_ = adminHTTP.Shutdown(shutdownCtx)
		_ = proxyHTTP.Shutdown(shutdownCtx)
		_ = proxyHTTPS.Shutdown(shutdownCtx)
	}()

	if err := g.Wait(); err != nil {
		logger.Error("Server stopped with error: %v", err)
	}
}

// setupWebHandler serves static files from web.DistFS or disk with SPA fallback to index.html
func setupWebHandler() http.Handler {
	// Try using embedded filesystem first
	distSubFS, err := fs.Sub(web.DistFS, "dist")
	if err == nil {
		if _, statErr := distSubFS.Open("index.html"); statErr == nil {
			fileServer := http.FileServer(http.FS(distSubFS))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				cleanPath := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
				if cleanPath == "" {
					cleanPath = "index.html"
				}

				f, openErr := distSubFS.Open(cleanPath)
				if openErr == nil {
					_ = f.Close()
					fileServer.ServeHTTP(w, r)
					return
				}

				// SPA fallback
				indexF, openErr := distSubFS.Open("index.html")
				if openErr == nil {
					_ = indexF.Close()
					r.URL.Path = "/"
					fileServer.ServeHTTP(w, r)
					return
				}
				http.NotFound(w, r)
			})
		}
	}

	// Fallback to disk if running from source without embedded assets
	distPath := "./web/dist"
	if _, err := os.Stat(distPath); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><h2>ProxyServer Gateway Active</h2></body></html>`))
		})
	}

	fileServer := http.FileServer(http.Dir(distPath))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := filepath.Clean(r.URL.Path)
		targetFile := filepath.Join(distPath, cleanPath)
		if fi, err := os.Stat(targetFile); err == nil && !fi.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(distPath, "index.html"))
	})
}

