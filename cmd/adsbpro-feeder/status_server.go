package main

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
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
	listenAddresses, err := resolveStatusListenAddresses(listenAddress)
	if err != nil {
		log.Printf("status page configuration %s is invalid: %v", listenAddress, err)
		return
	}
	servers := make([]*http.Server, 0, len(listenAddresses))
	for _, address := range listenAddresses {
		listener, listenErr := net.Listen("tcp", address)
		if listenErr != nil {
			log.Printf("status page unavailable on %s: %v", address, listenErr)
			continue
		}
		server := &http.Server{
			Handler: statusHandler(monitor), ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024,
		}
		servers = append(servers, server)
		log.Printf("status page listening on http://%s", address)
		go func() {
			if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
				log.Printf("status page stopped on %s: %v", address, serveErr)
			}
		}()
	}
	if len(servers) == 0 {
		return
	}
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, server := range servers {
		_ = server.Shutdown(shutdownCtx)
	}
}

func resolveStatusListenAddresses(value string) ([]string, error) {
	host, port, err := net.SplitHostPort(value)
	if err != nil || !strings.EqualFold(host, "private") {
		return []string{value}, err
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	addresses := map[string]struct{}{
		net.JoinHostPort("127.0.0.1", port): {},
		net.JoinHostPort("::1", port):       {},
	}
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 {
			continue
		}
		interfaceAddresses, addressErr := networkInterface.Addrs()
		if addressErr != nil {
			continue
		}
		for _, interfaceAddress := range interfaceAddresses {
			prefix, parseErr := netip.ParsePrefix(interfaceAddress.String())
			if parseErr != nil {
				continue
			}
			address := prefix.Addr().Unmap()
			if statusAddressIsPrivate(address) {
				addresses[net.JoinHostPort(address.String(), port)] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(addresses))
	for address := range addresses {
		result = append(result, address)
	}
	sort.Strings(result)
	return result, nil
}

func statusAddressIsPrivate(address netip.Addr) bool {
	if address.IsLoopback() || address.IsPrivate() || (address.Is4() && address.IsLinkLocalUnicast()) {
		return true
	}
	tailscale, _ := netip.ParsePrefix("100.64.0.0/10")
	return address.Is4() && tailscale.Contains(address)
}
