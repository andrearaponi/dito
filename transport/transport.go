package transport

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"dito/config"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
)

// Caronte is a custom HTTP transport that handles header manipulation and certificate-based TLS.
type Caronte struct {
	Location       *config.LocationConfig
	TransportCache *TransportCache
}

// TransportCache is a thread-safe cache for storing and retrieving custom HTTP transports.
type TransportCache struct {
	transports       sync.Map // Changed from map to sync.Map
	genericTransport *http.Transport
}

// NewTransportCache creates a new instance of TransportCache with a generic transport configuration.
//
// Parameters:
// - transportConfig: The configuration for the generic HTTP transport.
//
// Returns:
// - *TransportCache: A pointer to the newly created TransportCache.
func NewTransportCache(transportConfig config.HTTPTransportConfig) *TransportCache {
	genericTransport, err := createTransportFromConfig(transportConfig)
	if err != nil {
		log.Fatalf("Failed to create generic transport: %v", err)
	}
	return &TransportCache{
		transports:       sync.Map{},
		genericTransport: genericTransport,
	}
}

// GetTransport retrieves a custom HTTP transport for the given location configuration.
// If the transport does not exist, it creates a new one and stores it in the cache.
//
// Parameters:
// - location: The configuration for the location, including headers to manipulate and certificates.
// - genericTransportConfig: The global transport configuration.
//
// Returns:
// - *http.Transport: The custom HTTP transport.
// - error: An error if the custom transport could not be created.
func (c *TransportCache) GetTransport(location *config.LocationConfig, genericTransportConfig config.HTTPTransportConfig) (*http.Transport, error) {
	//log.Printf("Getting transport for location: %s\n", location.Path)
	var transportConfig config.HTTPTransportConfig
	if location.Transport != nil {
		transportConfig = location.Transport.HTTP
	} else {
		transportConfig = genericTransportConfig
	}

	key := generateTransportKey(transportConfig)

	// Attempt to load the transport from the map
	if value, ok := c.transports.Load(key); ok {
		// Type assertion
		transport, ok := value.(*http.Transport)
		if !ok {
			return nil, fmt.Errorf("invalid transport type")
		}
		return transport, nil
	}

	// Create the transport without a global lock
	customTransport, err := createTransportFromConfig(transportConfig)
	if err != nil {
		return nil, err
	}

	// Atomically load or store the transport
	actual, _ := c.transports.LoadOrStore(key, customTransport)
	return actual.(*http.Transport), nil
}

// InvalidateTransport removes the transport associated with the given configuration from the cache.
func (c *TransportCache) InvalidateTransport(transportConfig config.HTTPTransportConfig) {
	key := generateTransportKey(transportConfig)
	c.transports.Delete(key)
}

// Clear removes all transports from the cache.
func (c *TransportCache) Clear() {
	c.transports.Range(func(key, value interface{}) bool {
		c.transports.Delete(key)
		return true
	})
}

// RoundTrip executes a single HTTP transaction, manipulating headers and handling TLS certificates.
func (t *Caronte) RoundTrip(req *http.Request) (*http.Response, error) {
	// Use the custom or generic transport based on location configuration
	transport, err := t.TransportCache.GetTransport(t.Location, config.GetCurrentProxyConfig().Transport.HTTP)
	if err != nil {
		return nil, err
	}

	return transport.RoundTrip(req)
}

// createTransportFromConfig creates an HTTP transport based on the provided configuration and SSL settings.
//
// Parameters:
// - config: The HTTP transport configuration.
//
// Returns:
// - *http.Transport: A pointer to the created HTTP transport.
// - error: An error if the transport could not be created.
func createTransportFromConfig(config config.HTTPTransportConfig) (*http.Transport, error) {
	tlsConfig := &tls.Config{}

	if config.CertFile != "" && config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load key pair: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if config.CaFile != "" {
		caCert, err := os.ReadFile(config.CaFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %v", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	return &http.Transport{
		IdleConnTimeout:       config.IdleConnTimeout,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		MaxConnsPerHost:       config.MaxConnsPerHost,
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
		ExpectContinueTimeout: config.ExpectContinueTimeout,
		DisableCompression:    config.DisableCompression,
		ForceAttemptHTTP2:     config.ForceHTTP2,
		TLSClientConfig:       tlsConfig,
		DialContext: (&net.Dialer{
			Timeout:   config.DialTimeout,
			KeepAlive: config.KeepAlive,
		}).DialContext,
	}, nil
}

// generateTransportKey generates a unique key for the transport configuration.
func generateTransportKey(config config.HTTPTransportConfig) string {
	configBytes, _ := json.Marshal(config)
	hash := sha256.Sum256(configBytes)
	return fmt.Sprintf("%x", hash)
}

// printTransportDetails logs detailed information about the given HTTP transport.
// You can remove or comment out this function in production code.
func printTransportDetails(transport *http.Transport) {
	fmt.Printf("IdleConnTimeout: %s\n", transport.IdleConnTimeout)
	fmt.Printf("MaxIdleConns: %d\n", transport.MaxIdleConns)
	fmt.Printf("MaxIdleConnsPerHost: %d\n", transport.MaxIdleConnsPerHost)
	fmt.Printf("TLSHandshakeTimeout: %s\n", transport.TLSHandshakeTimeout)
	fmt.Printf("ResponseHeaderTimeout: %s\n", transport.ResponseHeaderTimeout)
	fmt.Printf("ExpectContinueTimeout: %s\n", transport.ExpectContinueTimeout)
	fmt.Printf("DisableCompression: %v\n", transport.DisableCompression)
	fmt.Printf("TLSClientConfig: %+v\n", transport.TLSClientConfig)
}
