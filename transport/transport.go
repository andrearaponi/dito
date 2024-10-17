package transport

import (
	"crypto/tls"
	"crypto/x509"
	"dito/config"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	XForwardedFor   = "X-Forwarded-For"
	XForwardedProto = "X-Forwarded-Proto"
	XForwardedHost  = "X-Forwarded-Host"
)

// Caronte is a custom HTTP transport that handles header manipulation and certificate-based TLS.
type Caronte struct {
	Location       *config.LocationConfig
	TransportCache *TransportCache
}

// TransportCache is a thread-safe cache for storing and retrieving custom HTTP transports.
type TransportCache struct {
	transports map[string]*http.Transport
	mu         sync.RWMutex
}

// NewTransportCache creates a new instance of TransportCache.
func NewTransportCache() *TransportCache {
	return &TransportCache{
		transports: make(map[string]*http.Transport),
	}
}

// GetTransport retrieves a custom HTTP transport for the given location configuration.
// If the transport does not exist, it creates a new one and stores it in the cache.
//
// Parameters:
// - location: The configuration for the location, including headers to manipulate and certificates.
//
// Returns:
// - *http.Transport: The custom HTTP transport.
// - error: An error if the custom transport could not be created.
func (c *TransportCache) GetTransport(location *config.LocationConfig) (*http.Transport, error) {
	c.mu.RLock()
	transport, ok := c.transports[location.Path]
	c.mu.RUnlock()

	if ok {
		return transport, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if transport, ok := c.transports[location.Path]; ok {
		return transport, nil
	}

	customTransport, err := createCustomTransport(location)
	if err != nil {
		return nil, err
	}

	c.transports[location.Path] = customTransport
	return customTransport, nil
}

// InvalidateTransport removes the transport associated with the given location path from the cache.
func (c *TransportCache) InvalidateTransport(locationPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.transports, locationPath)
}

// Clear removes all transports from the cache.
func (c *TransportCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.transports = make(map[string]*http.Transport)
}

// RoundTrip executes a single HTTP transaction, manipulating headers and handling TLS certificates.
func (t *Caronte) RoundTrip(req *http.Request) (*http.Response, error) {
	transport, err := t.TransportCache.GetTransport(t.Location)
	if err != nil {
		return nil, err
	}

	t.AddHeaders(req)

	return transport.RoundTrip(req)
}

// AddHeaders manipulates the request headers according to the LocationConfig.
//
// Parameters:
// - req: The HTTP request whose headers will be manipulated.
func (t *Caronte) AddHeaders(req *http.Request) {
	// Remove excluded headers
	for _, header := range t.Location.ExcludedHeaders {
		req.Header.Del(header)
	}

	// Add or modify headers specified in the configuration
	for header, value := range t.Location.AdditionalHeaders {
		req.Header.Set(header, value)
	}

	// Set the Host header, if specified
	if hostHeader, ok := t.Location.AdditionalHeaders["Host"]; ok {
		req.Host = hostHeader
	}

	// Add or preserve the X-Forwarded-* headers
	if !contains(t.Location.ExcludedHeaders, XForwardedFor) {
		clientIP := req.RemoteAddr
		if prior, ok := req.Header[XForwardedFor]; ok {
			req.Header.Set(XForwardedFor, prior[0]+", "+clientIP)
		} else {
			req.Header.Set(XForwardedFor, clientIP)
		}
	}

	if !contains(t.Location.ExcludedHeaders, XForwardedProto) {
		req.Header.Set(XForwardedProto, req.URL.Scheme)
	}

	if !contains(t.Location.ExcludedHeaders, XForwardedHost) {
		req.Header.Set(XForwardedHost, req.Host)
	}
}

// createCustomTransport creates a custom HTTP transport based on the provided certificate files.
//
// Parameters:
// - location: The configuration for the location, including headers to manipulate and certificates.
//
// Returns:
// - *http.Transport: The custom HTTP transport.
// - error: An error if the custom transport could not be created.
func createCustomTransport(location *config.LocationConfig) (*http.Transport, error) {
	tlsConfig := &tls.Config{}

	// Load CA certificate if specified
	if location.CaFile != "" {
		caCert, err := os.ReadFile(location.CaFile)
		if err != nil {
			return nil, fmt.Errorf("errore nella lettura del CA file: %w", err)
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate and key if specified
	if location.CertFile != "" && location.KeyFile != "" {
		clientCert, err := tls.LoadX509KeyPair(location.CertFile, location.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("errore nel caricamento del certificato/chiave del client: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	// Return a new HTTP transport with the custom TLS config
	return &http.Transport{
		TLSClientConfig:       tlsConfig,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}, nil
}

// contains checks if a header is in the list of excluded headers.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
