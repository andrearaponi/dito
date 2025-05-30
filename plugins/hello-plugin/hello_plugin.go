package main

import (
	"context"
	"dito/plugin" // Import Dito's plugin package
	"log/slog"
	"net/http"
)

// HelloPlugin is our simple plugin implementation
type HelloPlugin struct {
	logger *slog.Logger
	config map[string]interface{}
}

// NewPlugin is the required function that Dito calls to create a plugin instance
// This function MUST be exported and return plugin.Plugin
func NewPlugin() plugin.Plugin {
	return &HelloPlugin{}
}

// Name returns the plugin's unique identifier
func (p *HelloPlugin) Name() string {
	return "hello-plugin"
}

// Init initializes the plugin with configuration and app accessor
func (p *HelloPlugin) Init(ctx context.Context, config map[string]interface{}, appAccessor plugin.AppAccessor) error {
	p.logger = appAccessor.GetLogger()
	p.config = config

	// Log configuration received
	p.logger.Debug("HelloPlugin: Initializing with config", "config", config)

	// Check if plugin is enabled
	if enabled, ok := config["plugin_enabled"].(bool); ok && !enabled {
		p.logger.Info("Hello plugin is disabled")
		return nil
	}

	// Read custom greeting if available
	if greeting, ok := config["greeting_message"].(string); ok {
		p.logger.Info("Hello plugin initialized!", "custom_greeting", greeting)
	} else {
		p.logger.Info("Hello plugin initialized successfully!")
	}

	return nil
}

// MiddlewareFunc returns the HTTP middleware function
func (p *HelloPlugin) MiddlewareFunc() func(http.Handler) http.Handler {
	// Get greeting message from config or use default
	greeting := "Hello from Dito plugin!"
	if msg, ok := p.config["greeting_message"].(string); ok {
		greeting = msg
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Log our hello message
			p.logger.Info("Hello from plugin!",
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
			)

			// Add a custom header
			w.Header().Set("X-Hello-Plugin", greeting)

			// Continue with the next handler
			next.ServeHTTP(w, r)
		})
	}
}
