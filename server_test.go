package proxytv

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerStartStop(t *testing.T) {
	// Create a mock provider
	provider := &Provider{}

	// Create a config
	config := &Config{
		ListenAddress: ":8080",
		UseFFMPEG:     true,
		MaxStreams:    10,
	}

	// Create a new server
	server, err := NewServer(config, provider)
	require.NoError(t, err)

	// Start the server
	errChan := server.Start(provider)

	// Create a test HTTP client
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// Test the /ping endpoint
	t.Run("Ping", func(t *testing.T) {
		resp, err := client.Get("http://localhost:8080/ping")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "PONG", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Stop the server
	err = server.Stop()
	require.NoError(t, err)

	// Check if there were any errors during server run
	select {
	case err := <-errChan:
		assert.NoError(t, err)
	default:
	}
}
