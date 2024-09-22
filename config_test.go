package proxytv

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	// Test with valid configuration
	t.Run("Valid Configuration", func(t *testing.T) {
		content := []byte(`
logLevel: info
iptvUrl: http://example.com/iptv
epgUrl: http://example.com/epg
listenAddress: :8080
baseAddress: http://localhost:8080
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
		assert.Equal(t, ":8080", config.ListenAddress)
		assert.Equal(t, "http://localhost:8080", config.BaseAddress)
		assert.True(t, config.UseFFMPEG)
		assert.Equal(t, 10, config.MaxStreams)
		assert.Len(t, config.Filters, 2)
		assert.Equal(t, "sports.*", config.Filters[0].Value)
		assert.Equal(t, "include", config.Filters[0].Type)
		assert.NotNil(t, config.Filters[0].GetRegexp())
		assert.Equal(t, "news|weather", config.Filters[1].Value)
		assert.Equal(t, "exclude", config.Filters[1].Type)
		assert.NotNil(t, config.Filters[1].GetRegexp())
	})

	// Test with invalid regular expression
	t.Run("Invalid Regular Expression", func(t *testing.T) {
		content := []byte(`
logLevel: info
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
}
