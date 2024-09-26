package main

import (
	"flag"
	"fmt"
	"net/url"
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

var (
	safeURLRegex       = regexp.MustCompile(`(?m)(username|password|token)=[\w=]+(&?)`)
	safeURLReplaceFunc = func(input string) string {
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

type safeURLHook struct{}

func (h *safeURLHook) Levels() []log.Level {
	return log.AllLevels
}

func (h *safeURLHook) Fire(entry *log.Entry) error {
	if urlStr, ok := entry.Data["url"]; ok {
		urlStr = safeURLRegex.ReplaceAllStringFunc(urlStr.(string), safeURLReplaceFunc)
		if url, err := url.Parse(urlStr.(string)); err == nil {
			pathParts := strings.Split(url.Path, "/")[1:]
			for i := 0; len(pathParts) > 1 && i < len(pathParts)-1; i++ {
				pathParts[i] = "XXXX"
			}
			url.Path = "/" + strings.Join(pathParts, "/")
			urlStr = url.String()
		}
		entry.Data["url"] = urlStr
	}
	return nil
}

func initLogging() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true, // Set to true to include full timestamp
	})
	log.AddHook(&safeURLHook{})
}

func init() {
	initLogging()

	proxytv.SetGinMode()
}

func main() {
	// Define command-line flag for config file
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

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

	server, err := proxytv.NewServer(config, provider, gitCommit)
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
