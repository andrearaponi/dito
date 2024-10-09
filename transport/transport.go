package transport

import (
	"crypto/tls"
	"crypto/x509"
	"dito/config"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	XForwardedFor   = "X-Forwarded-For"
	XForwardedProto = "X-Forwarded-Proto"
	XForwardedHost  = "X-Forwarded-Host"
)

// Caronte is a unified custom transport for HTTP client, handling both header manipulation and certificate-based TLS.
type Caronte struct {
	RT       http.RoundTripper      // The underlying RoundTripper to execute requests.
	Location *config.LocationConfig // Configuration for the location, including headers to manipulate and certificates.
}

// RoundTrip executes a single HTTP transaction, manipulates headers, and handles TLS certificates.
//
// Parameters:
// - req: The HTTP request to be executed.
//
// Returns:
// - *http.Response: The HTTP response received.
// - error: An error if the request failed.
func (t *Caronte) RoundTrip(req *http.Request) (*http.Response, error) {
	// Get the latest configuration
	currentConfig := config.GetCurrentProxyConfig()

	// Dynamically update the Location with the latest config
	for i, loc := range currentConfig.Locations {
		if loc.Path == t.Location.Path {
			t.Location = &currentConfig.Locations[i]
			break
		}
	}

	// Handle certificate-based TLS if necessary
	var transport http.RoundTripper = t.RT
	if t.Location.CertFile != "" || t.Location.KeyFile != "" || t.Location.CaFile != "" {
		customTransport, err := t.CreateCustomTransport()
		if err != nil {
			return nil, fmt.Errorf("failed to create custom transport: %w", err)
		}
		transport = customTransport
	}

	// Manipulate headers before forwarding the request
	t.AddHeaders(req)

	// Execute the request and capture the response
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
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

// CreateCustomTransport creates a custom HTTP transport based on the provided certificate files.
//
// Returns:
// - *http.Transport: The custom HTTP transport.
// - error: An error if the custom transport could not be created.
func (t *Caronte) CreateCustomTransport() (*http.Transport, error) {
	tlsConfig := &tls.Config{}

	// Load CA certificate
	if t.Location.CaFile != "" {
		caCert, err := ioutil.ReadFile(t.Location.CaFile)
		if err != nil {
			return nil, fmt.Errorf("error reading CA file: %w", err)
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate and key
	if t.Location.CertFile != "" && t.Location.KeyFile != "" {
		clientCert, err := tls.LoadX509KeyPair(t.Location.CertFile, t.Location.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("error loading client certificate/key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	// Return a new HTTP transport with the custom TLS config
	return &http.Transport{
		TLSClientConfig:       tlsConfig,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          10,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}, nil
}

// contains checks if a header is in the list of excluded headers.
//
// Parameters:
// - slice: The list of headers.
// - item: The header to check.
//
// Returns:
// - bool: True if the header is in the list, false otherwise.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
