package proxytv

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
)

type Filter struct {
	Value  string         `yaml:"filter"`
	Type   string         `yaml:"type"`
	regexp *regexp.Regexp // Compiled regular expression
}

// GetRegexp returns the compiled regular expression
func (f *Filter) GetRegexp() *regexp.Regexp {
	return f.regexp
}

type Config struct {
	LogLevel string `yaml:"logLevel,omitempty" default:"info"`
	IPTVUrl  string `yaml:"iptvUrl"`
	EPGUrl   string `yaml:"epgUrl"`

	ListenAddress string `yaml:"listenAddress,omitempty" default:"0.0.0.0:6078"`
	ServerAddress string `yaml:"serverAddress"`

	UseFFMPEG  bool `yaml:"ffmpeg,omitempty" default:"true"`
	MaxStreams int  `yaml:"maxStreams,omitempty" default:"1"`

	RefreshInterval    time.Duration
	RefreshIntervalStr string `yaml:"refreshInterval,omitempty" default:"12h"`

	Filters []*Filter `yaml:"filters"`
}

// LoadConfig reads a YAML config file from the given path and returns a Config pointer.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &Config{}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	if err = defaults.Set(config); err != nil {
		return nil, err
	}

	config.RefreshInterval, err = time.ParseDuration(config.RefreshIntervalStr)
	if err != nil {
		return nil, fmt.Errorf("invalid refreshInterval: %w", err)
	}

	if config.IPTVUrl == "" {
		return nil, fmt.Errorf("iptvUrl is required")
	}
	if config.EPGUrl == "" {
		return nil, fmt.Errorf("epgUrl is required")
	}
	if config.ServerAddress == "" {
		return nil, fmt.Errorf("serverAddress is required")
	}

	re := regexp.MustCompile(`^https?://`)
	config.ServerAddress = re.ReplaceAllString(config.ServerAddress, "")

	if err := validateFileOrURL(config.IPTVUrl); err != nil {
		return nil, fmt.Errorf("invalid iptvUrl: %w", err)
	}
	if err := validateFileOrURL(config.EPGUrl); err != nil {
		return nil, fmt.Errorf("invalid epgUrl: %w", err)
	}

	if config.UseFFMPEG {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			return nil, fmt.Errorf("ffmpeg is enabled but not found in PATH: %w", err)
		}
	}

	if err := config.compileFilterRegexps(); err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) compileFilterRegexps() error {
	for i, filter := range c.Filters {
		re, err := regexp.Compile(filter.Value)
		if err != nil {
			return fmt.Errorf("invalid regular expression in filter %d: %w", i, err)
		}
		c.Filters[i].regexp = re
	}
	return nil
}

func validateFileOrURL(input string) error {
	// Check if it's a file
	if _, err := os.Stat(input); err == nil {
		return nil
	}

	// Check if it's a URL
	if matched, _ := regexp.MatchString(`^https?://`, input); matched {
		return nil
	}

	return fmt.Errorf("not a valid file path or URL")
}
