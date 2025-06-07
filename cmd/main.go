package main

import (
	"context"
	"crypto/tls"
	"dito/app"
	"dito/config"
	"dito/handlers"
	"dito/logging"
	"dito/metrics"
	cmid "dito/middlewares"
	"dito/plugin"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

// main is the entry point of the application.
// It loads the configuration, initializes the logger and Redis client, and starts the HTTP server.
func main() {
	// Define a flag to enable the profiler
	enableProfiler := flag.Bool("enable-profiler", false, "enable the profiler")

	// Define a flag for the configuration file path
	configFile := flag.String("f", "config.yaml", "path to the configuration file")
	flag.Parse()

	// Check if the configuration file exists
	if _, err := os.Stat(*configFile); os.IsNotExist(err) {
		log.Fatalf("Configuration file not found: %s", *configFile)
	}

	// Load and set the configuration
	config.LoadAndSetConfig(*configFile)
	logger := logging.InitializeLogger(config.GetCurrentProxyConfig().Logging.Level)

	// Initialize metrics system
	metrics.InitMetrics()

	transportConfig := &config.GetCurrentProxyConfig().Transport.HTTP

	// Create a new Dito instance
	dito := app.NewDito(transportConfig, logger)

	// Define a callback function to handle configuration changes
	onChange := func(newConfig *config.ProxyConfig) {
		// Update components with the new configuration
		dito.UpdateComponents(newConfig)
		// Update the Dito instance configuration
		dito.UpdateConfig(newConfig)
	}

	// Watch the configuration file for changes if hot reload is enabled
	if dito.GetCurrentConfig().HotReload {
		go config.WatchConfig(*configFile, onChange, logger)
	}

	// Start the profiler if the flag is enabled
	if *enableProfiler {
		startProfiling(dito.Logger)
	}

	// Start the HTTP server
	StartServer(dito)
}

// StartServer initializes and starts the HTTP server for the Dito application.
// It sets up the necessary routes, handles graceful shutdown on receiving OS interrupt signals,
// and logs the server status.
//
// Parameters:
//
//	dito (*app.Dito): The Dito application instance containing configuration and logger.
func StartServer(dito *app.Dito) {
	var err error

	plugins, pluginConfigs, err := plugin.LoadAndVerifyPlugins()
	if err != nil {
		// Use the logger from the Dito app instance if available, otherwise slog.Default()
		var currentLogger *slog.Logger = slog.Default()
		if dito != nil && dito.Logger != nil {
			currentLogger = dito.Logger
		}
		currentLogger.Error("Error loading plugins", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize each verified plugin manually
	for _, p := range plugins {
		dito.Logger.Info("Initializing plugin", slog.String("plugin_name", p.Name()))

		// DEBUG: Check if the configuration is still present
		pluginSpecificConfig, exists := pluginConfigs[p.Name()] // Renamed 'config' to avoid conflict
		dito.Logger.Debug("Plugin configuration before Init",
			slog.String("plugin_name", p.Name()),
			slog.Any("config_data", pluginSpecificConfig))

		if !exists {
			pluginSpecificConfig = make(map[string]interface{}) // If not exists, pass an empty configuration
			dito.Logger.Debug("Plugin configuration not found, using empty default.", slog.String("plugin_name", p.Name()))
		}

		// Initialize the plugin with its specific configuration
		if err := p.Init(context.Background(), pluginSpecificConfig, dito); err != nil {
			dito.Logger.Error("Failed to initialize plugin",
				slog.String("plugin_name", p.Name()),
				slog.Any("error", err))
			os.Exit(1) // If a critical plugin fails, block startup
		}
	}

	// Create a new HTTP request multiplexer (mux) to handle incoming requests.
	mux := http.NewServeMux()

	mux.Handle("/", cmid.LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.DynamicProxyHandler(dito, w, r, plugins)
	}), dito))

	// Create a custom HTTP server with the specified address and handler.
	server := &http.Server{
		Addr:    ":" + dito.Config.Port,
		Handler: mux,
	}

	if dito.Config.CertFile != "" && dito.Config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(dito.Config.CertFile, dito.Config.KeyFile)
		if err != nil {
			dito.Logger.Error("Failed to load server key pair", "error", err)
			os.Exit(1)
		}

		tlsConfig := &tls.Config{}
		tlsConfig.Certificates = []tls.Certificate{cert}
		tlsConfig.MinVersion = tls.VersionTLS12

		server.TLSConfig = tlsConfig
	}

	// Channel to listen for OS interrupt signals (e.g., Ctrl+C).
	idleConnsClosed := make(chan struct{})

	go func() {
		// Listen for interrupt signals.
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		// Signal received, initiate graceful shutdown.
		dito.Logger.Info("Shutting down server gracefully...")

		// Context with timeout for graceful shutdown (e.g., 30 seconds).
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Attempt to gracefully shut down the server.
		if err := server.Shutdown(ctx); err != nil {
			dito.Logger.Error("Server forced to shutdown", "error", err)
		} else {
			dito.Logger.Info("Server shut down gracefully.")
		}

		close(idleConnsClosed)
	}()

	// Log server start message.
	dito.Logger.Info("Dito is ready", slog.String("port", dito.Config.Port))

	// Start the HTTP server.
	if server.TLSConfig != nil {
		err = server.ListenAndServeTLS("", "")
	} else {
		err = server.ListenAndServe()
	}
	if !errors.Is(err, http.ErrServerClosed) {
		dito.Logger.Error("Server failed to start", "error", err)
		log.Fatal(err)
	}

	// Wait for all idle connections to close.
	<-idleConnsClosed
	dito.Logger.Info("All connections closed, exiting.")
}

// startProfiling enables various runtime profiling options and starts the pprof server.
func startProfiling(logger *slog.Logger) {
	// Start the profiling server for performance analysis
	go func() {
		logger.Info("Starting pprof on :6060")
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			logger.Error("pprof failed", "error", err)
		}
	}()

	// Enable mutex profiling to help identify lock contention issues
	runtime.SetMutexProfileFraction(1)

	// Enable block profiling to help identify blocking operations
	runtime.SetBlockProfileRate(1)
}
