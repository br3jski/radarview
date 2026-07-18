package main

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"time"
)

//go:embed web/*
var embeddedWeb embed.FS

func statusHandler(monitor *runtimeMonitor) http.Handler {
	mux := http.NewServeMux()
	webRoot, _ := fs.Sub(embeddedWeb, "web")
	assets := http.FileServer(http.FS(webRoot))
	mux.HandleFunc("/api/status", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(writer).Encode(monitor.snapshot())
	})
	mux.Handle("/", assets)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Allow", "GET, HEAD")
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Frame-Options", "DENY")
		mux.ServeHTTP(writer, request)
	})
}

func serveStatus(ctx context.Context, listenAddress string, monitor *runtimeMonitor) {
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Printf("status page unavailable on %s: %v", listenAddress, err)
		return
	}
	server := &http.Server{
		Handler: statusHandler(monitor), ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Printf("status page listening on http://%s", listenAddress)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Printf("status page stopped: %v", err)
	}
}
