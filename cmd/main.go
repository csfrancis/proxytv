package main

import (
	"flag"

	"github.com/csfrancis/proxytv"

	log "github.com/sirupsen/logrus"
)

func main() {
	// Define command-line flag for config file
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true, // Set to true to include full timestamp
	})

	// Use the provided config file path or the default
	config, err := proxytv.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logLevel, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		log.Warnf("invalid log level %q, defaulting to info", config.LogLevel)
		logLevel = log.InfoLevel
	}
	log.SetLevel(logLevel)

	log.Infof("starting proxytv with config file: %s", *configPath)

	provider, err := proxytv.NewProvider(config)
	if err != nil {
		log.Fatalf("failed to create provider: %v", err)
	}

	err = provider.Load()
	if err != nil {
		log.Fatalf("failed to load provider: %v", err)
	}
}
