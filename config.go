package proxytv

import (
	"fmt"
	"os"
	"regexp"

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
	LogLevel string `yaml:"logLevel"`
	IPTVUrl  string `yaml:"iptvUrl"`
	EPGUrl   string `yaml:"epgUrl"`

	ListenAddress string `yaml:"listenAddress"`
	BaseAddress   string `yaml:"baseAddress"`

	UseFFMPEG  bool `yaml:"ffmpeg"`
	MaxStreams int  `yaml:"maxStreams"`

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

	// Compile regular expressions for filters
	for i, filter := range config.Filters {
		re, err := regexp.Compile(filter.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid regular expression in filter %d: %w", i, err)
		}
		config.Filters[i].regexp = re
	}

	return config, nil
}
