package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type NodeDiscoveryInterface interface {
	GetCurrentNodeIP(ctx context.Context) (string, error)
}

type Handler struct {
	nodeDiscovery NodeDiscoveryInterface
	client        *http.Client
}

func NewHandler(nodeDiscovery NodeDiscoveryInterface) *Handler {
	return &Handler{
		nodeDiscovery: nodeDiscovery,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" {
		h.handleHealth(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	nodeIP, err := h.nodeDiscovery.GetCurrentNodeIP(ctx)
	if err != nil {
		log.Printf("Failed to discover node IP: %v", err)
		http.Error(w, "Failed to discover target node", http.StatusServiceUnavailable)
		return
	}

	port := h.extractPort(r.Host)
	targetURL := fmt.Sprintf("http://%s:%s%s", nodeIP, port, r.URL.Path)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	log.Printf("Proxying %s %s -> %s", r.Method, r.URL.String(), targetURL)

	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL, r.Body)
	if err != nil {
		log.Printf("Failed to create proxy request: %v", err)
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	for key, values := range r.Header {
		if !h.shouldSkipHeader(key) {
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}
	}

	resp, err := h.client.Do(proxyReq)
	if err != nil {
		log.Printf("Failed to proxy request: %v", err)
		http.Error(w, "Failed to proxy request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nodeIP, err := h.nodeDiscovery.GetCurrentNodeIP(ctx)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "UNHEALTHY: %v\n", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK: Forwarding to node %s\n", nodeIP)
}

func (h *Handler) extractPort(host string) string {
	if strings.Contains(host, ":") {
		_, port, err := parseHostPort(host)
		if err == nil && port != "" {
			return port
		}
	}
	return "80"
}

func parseHostPort(hostPort string) (host, port string, err error) {
	colon := strings.LastIndexByte(hostPort, ':')
	if colon == -1 {
		return hostPort, "", nil
	}
	if i := strings.LastIndexByte(hostPort, ']'); i != -1 {
		if colon <= i {
			return hostPort, "", nil
		}
	}
	host, port = hostPort[:colon], hostPort[colon+1:]
	if _, err := strconv.Atoi(port); err != nil {
		return "", "", fmt.Errorf("invalid port: %s", port)
	}
	return host, port, nil
}

func (h *Handler) shouldSkipHeader(key string) bool {
	key = strings.ToLower(key)
	return key == "connection" || key == "upgrade" || key == "proxy-connection" || key == "proxy-authenticate" || key == "proxy-authorization" || key == "te" || key == "trailers" || key == "transfer-encoding"
}
