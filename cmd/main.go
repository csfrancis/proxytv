package main

import (
	"github.com/csfrancis/proxytv"

	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetFormatter(
		&log.TextFormatter{
			FullTimestamp: true, // Set to true to include full timestamp
		})

	log.Info("starting proxytv")

	config, err := proxytv.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Infof("config: %v", config)
}
