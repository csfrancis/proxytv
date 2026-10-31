package proxytv

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
	server, err := NewServer(config, provider, "test")
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

func TestServerDisconnectStream(t *testing.T) {
	server, err := NewServer(&Config{MaxStreams: 10}, &Provider{}, "test")
	require.NoError(t, err)

	// Stand in for the ffmpeg-backed channel handler: stream until the
	// request context is cancelled, then report why.
	causes := make(chan error, 1)
	server.router.Use(server.streamTracker)
	server.router.GET(channelURIPrefix+":channelId", func(c *gin.Context) {
		c.Status(http.StatusOK)
		c.Writer.Flush()
		<-c.Request.Context().Done()
		causes <- context.Cause(c.Request.Context())
	})
	server.router.DELETE("/streams/:streamId", server.disconnectStream())

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	streamResp, err := http.Get(ts.URL + channelURIPrefix + "3")
	require.NoError(t, err)
	defer streamResp.Body.Close()

	var streams []streamInfo
	require.Eventually(t, func() bool {
		streams = server.getActiveStreams()
		return len(streams) == 1
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, 3, streams[0].ChannelID)

	t.Run("invalid id", func(t *testing.T) {
		resp := doDelete(t, ts.URL+"/streams/abc")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("unknown id", func(t *testing.T) {
		resp := doDelete(t, fmt.Sprintf("%s/streams/%d", ts.URL, streams[0].ID+100))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Len(t, server.getActiveStreams(), 1)
	})

	t.Run("active stream", func(t *testing.T) {
		resp := doDelete(t, fmt.Sprintf("%s/streams/%d", ts.URL, streams[0].ID))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "No active streams")

		select {
		case cause := <-causes:
			assert.ErrorIs(t, cause, errStreamDisconnected)
		case <-time.After(2 * time.Second):
			t.Fatal("stream was not cancelled")
		}

		// The client sees the stream end.
		_, err = io.ReadAll(streamResp.Body)
		assert.NoError(t, err)
		assert.Empty(t, server.getActiveStreams())
	})
}

func doDelete(t *testing.T, url string) *http.Response {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}
