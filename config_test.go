package proxytv

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	// Test with valid configuration
	t.Run("Valid Configuration", func(t *testing.T) {
		content := []byte(`
logLevel: info
iptvUrl: http://example.com/iptv
epgUrl: http://example.com/epg
listenAddress: localhost:8080
baseAddress: iptvserver:8080
refreshInterval: '2h'
ffmpeg: true
maxStreams: 10
filters:
  - filter: sports.*
    type: include
  - filter: news|weather
    type: exclude
`)

		tmpfile, err := os.CreateTemp("", "config*.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write(content); err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}
		if err := tmpfile.Close(); err != nil {
			t.Fatalf("Failed to close temp file: %v", err)
		}

		config, err := LoadConfig(tmpfile.Name())
		assert.NoError(t, err)
		assert.NotNil(t, config)

		assert.Equal(t, "info", config.LogLevel)
		assert.Equal(t, "http://example.com/iptv", config.IPTVUrl)
		assert.Equal(t, "http://example.com/epg", config.EPGUrl)
		assert.Equal(t, "localhost:8080", config.ListenAddress)
		assert.Equal(t, "iptvserver:8080", config.ServerAddress)
		assert.True(t, config.UseFFMPEG)
		assert.Equal(t, 10, config.MaxStreams)
		assert.Len(t, config.Filters, 2)
		assert.Equal(t, "sports.*", config.Filters[0].Value)
		assert.Equal(t, "include", config.Filters[0].Type)
		assert.NotNil(t, config.Filters[0].GetRegexp())
		assert.Equal(t, "news|weather", config.Filters[1].Value)
		assert.Equal(t, "exclude", config.Filters[1].Type)
		assert.NotNil(t, config.Filters[1].GetRegexp())
		assert.Equal(t, 2*time.Hour, config.RefreshInterval)
	})

	// Test with invalid regular expression
	t.Run("Invalid Regular Expression", func(t *testing.T) {
		content := []byte(`
logLevel: info
iptvUrl: http://example.com/iptv
epgUrl: http://example.com/iptv
filters:
  - filter: sports.*
    type: include
  - filter: news[
    type: exclude
`)

		tmpfile, err := os.CreateTemp("", "config*.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write(content); err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}
		if err := tmpfile.Close(); err != nil {
			t.Fatalf("Failed to close temp file: %v", err)
		}

		config, err := LoadConfig(tmpfile.Name())
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "invalid regular expression in filter 1")
	})

	// Test with invalid IPTV and EPG URLs
	t.Run("Invalid IPTV and EPG URLs", func(t *testing.T) {
		content := []byte(`
iptvUrl: invalid://example.com/iptv
epgUrl: not_a_valid_url
`)

		tmpfile, err := os.CreateTemp("", "config*.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write(content); err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}
		if err := tmpfile.Close(); err != nil {
			t.Fatalf("Failed to close temp file: %v", err)
		}

		config, err := LoadConfig(tmpfile.Name())
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "invalid iptvUrl")

		// Test with valid IPTV URL but invalid EPG URL
		content = []byte(`
iptvUrl: http://example.com/iptv
epgUrl: not_a_valid_url
`)

		if err := os.WriteFile(tmpfile.Name(), content, 0644); err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}

		config, err = LoadConfig(tmpfile.Name())
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "invalid epgUrl")
	})

	// Test with valid file paths for IPTV and EPG URLs
	t.Run("Valid file paths for IPTV and EPG URLs", func(t *testing.T) {
		// Create temporary IPTV file
		iptvFile, err := os.CreateTemp("", "iptv*.m3u")
		if err != nil {
			t.Fatalf("Failed to create temp IPTV file: %v", err)
		}
		defer os.Remove(iptvFile.Name())

		// Create temporary EPG file
		epgFile, err := os.CreateTemp("", "epg*.xml")
		if err != nil {
			t.Fatalf("Failed to create temp EPG file: %v", err)
		}
		defer os.Remove(epgFile.Name())

		// Create config content with file paths
		content := []byte(fmt.Sprintf(`
iptvUrl: %s
epgUrl: %s
`, iptvFile.Name(), epgFile.Name()))

		// Create temporary config file
		configFile, err := os.CreateTemp("", "config*.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp config file: %v", err)
		}
		defer os.Remove(configFile.Name())

		if _, err := configFile.Write(content); err != nil {
			t.Fatalf("Failed to write to temp config file: %v", err)
		}
		if err := configFile.Close(); err != nil {
			t.Fatalf("Failed to close temp config file: %v", err)
		}

		// Load the config
		config, err := LoadConfig(configFile.Name())
		assert.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, iptvFile.Name(), config.IPTVUrl)
		assert.Equal(t, epgFile.Name(), config.EPGUrl)
	})
}
