package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

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

	gitCommit string
)

func (h *SafeUrlHook) Fire(entry *log.Entry) error {
	if url, ok := entry.Data["url"]; ok {
		entry.Data["url"] = safeUrlRegex.ReplaceAllStringFunc(url.(string), safeUrlReplaceFunc)

	}
	return nil
}

func init() {
	proxytv.SetGinMode()
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

	log.WithFields(
		log.Fields{
			"gitCommit": gitCommit,
			"config":    *configPath,
		}).Info("starting proxytv")

	if config.UseFFMPEG {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			log.Fatalf("ffmpeg is enabled but not found in PATH: %v", err)
		}
	}

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

	errChan := server.Start(provider)

	refreshTicker := time.NewTicker(config.RefreshInterval)
	defer refreshTicker.Stop()

	exit := false

	for !exit {
		select {
		case err := <-errChan:
			log.Fatalf("server error: %v", err)
		case <-stop:
			log.Info("shutting down")
			exit = true
		case <-refreshTicker.C:
			log.Info("refreshing provider")
			if err := provider.Refresh(); err != nil {
				log.WithError(err).Error("failed to refresh provider")
			}
		}
	}

	server.Stop()
}
