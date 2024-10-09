package main

import (
	"dito/app"
	credis "dito/client/redis"
	"dito/config"
	"dito/handlers"
	"dito/logging"
	"flag"
	"fmt"
	"github.com/redis/go-redis/v9"
	"log"
	"net/http"
	"os"
)

// main is the entry point of the application.
// It loads the configuration, initializes the logger and Redis client, and starts the HTTP server.
func main() {
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

	var redisClient *redis.Client
	if config.GetCurrentProxyConfig().Redis.Enabled {
		// Initialize the Redis client
		var err error
		redisClient, err = credis.InitRedis(logger, config.GetCurrentProxyConfig().Redis)
		if err != nil {
			log.Fatal("Failed to initialize Redis client: ", err)
		}
	}

	// Create a new Dito instance
	dito := app.NewDito(redisClient, logger)

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

	// Start the HTTP server
	StartServer(dito)
}

// StartServer initializes the HTTP server and sets up the route handler.
// It logs the server start message and handles any errors that occur during server startup.
//
// Parameters:
// - dito: A pointer to the Dito instance containing the application configuration and logger.
func StartServer(dito *app.Dito) {
	// Create a new ServeMux
	mux := http.NewServeMux()
	// Set up the route handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handlers.DynamicProxyHandler(dito, w, r)
	})

	// Log the server start message
	dito.Logger.Info(fmt.Sprintf("👉 Dito it's ready on port: %s", dito.Config.Port))
	// Start the HTTP server
	err := http.ListenAndServe(":"+dito.Config.Port, mux)
	if err != nil {
		dito.Logger.Error("Server failed to start: ", err)
		log.Fatal(err)
	}
}
