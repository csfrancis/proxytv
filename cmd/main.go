package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/csfrancis/proxytv"

	log "github.com/sirupsen/logrus"
)

type SafeUrlHook struct{}

func (h *SafeUrlHook) Levels() []log.Level {
	return log.AllLevels
}

var (
	safeUrlRegex       = regexp.MustCompile(`(?m)(username|password|token)=[\w=]+(&?)`)
	safeUrlReplaceFunc = func(input string) string {
		ret := input
		if strings.HasPrefix(input, "username=") {
			ret = "username=REDACTED"
		} else if strings.HasPrefix(input, "password=") {
			ret = "password=REDACTED"
		} else if strings.HasPrefix(input, "token=") {
			ret = "token=REDACTED"
		}

		if strings.HasSuffix(input, "&") {
			return fmt.Sprintf("%s&", ret)
		}

		return ret
	}
)

func (h *SafeUrlHook) Fire(entry *log.Entry) error {
	if url, ok := entry.Data["url"]; ok {
		entry.Data["url"] = safeUrlRegex.ReplaceAllStringFunc(url.(string), safeUrlReplaceFunc)

	}
	return nil
}

func main() {
	// Define command-line flag for config file
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true, // Set to true to include full timestamp
	})
	log.AddHook(&SafeUrlHook{})

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

	err = provider.Refresh()
	if err != nil {
		log.Fatalf("failed to load provider: %v", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	server, err := proxytv.NewServer(config, provider)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	server.Start(provider)

	<-stop

	log.Info("shutting down")

	server.Stop()
}
