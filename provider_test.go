package proxytv

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTempFile(content string, pattern string) (*os.File, error) {
	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return nil, err
	}
	_, err = tmpFile.WriteString(content)
	if err != nil {
		tmpFile.Close()
		return nil, err
	}
	tmpFile.Close()
	return tmpFile, nil
}

func TestProviderLoad(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		m3uContent  string
		expectedM3u string
		epgContent  string
		wantErr     bool
	}{
		{
			name: "Valid M3U content",
			config: &Config{
				Filters: []*Filter{
					{Type: "id", Value: ".*"},
				},
			},
			m3uContent: `#EXTM3U
#EXTINF:-1 tvg-id="id1" tvg-name="name1",Channel 1
http://example.com/channel1
#EXTINF:-1 tvg-id="id2" tvg-name="name2",Channel 2
http://example.com/channel2`,
			expectedM3u: `#EXTM3U
#EXTINF:-1 tvg-id="id1" tvg-name="name1",Channel 1
http://example.com/channel1
#EXTINF:-1 tvg-id="id2" tvg-name="name2",Channel 2
http://example.com/channel2
`,
			epgContent: `<?xml version="1.0" encoding="ISO-8859-1"?>
<!DOCTYPE tv SYSTEM "xmltv.dtd">
`,
			wantErr: false,
		},
		{
			name: "Invalid M3U content",
			config: &Config{
				Filters: []*Filter{
					{Type: "id", Value: ".*"},
				},
			},
			m3uContent: "Invalid content",
			wantErr:    true,
		},
		{
			name: "Filtered valid M3U content",
			config: &Config{
				Filters: []*Filter{
					{Type: "id", Value: "id2"},
				},
			},
			m3uContent: `#EXTM3U
#EXTINF:-1 tvg-id="id1" tvg-name="name1",Channel 1
http://example.com/channel1
#EXTINF:-1 tvg-id="id2" tvg-name="name2",Channel 2
http://example.com/channel2`,
			expectedM3u: `#EXTM3U
#EXTINF:-1 tvg-id="id2" tvg-name="name2",Channel 2
http://example.com/channel2
`,
			epgContent: `<?xml version="1.0" encoding="ISO-8859-1"?>
<!DOCTYPE tv SYSTEM "xmltv.dtd">
`,
			wantErr: false,
		},
		{
			name: "Filtered valid M3U content",
			config: &Config{
				Filters: []*Filter{
					{Type: "id", Value: ".*"},
				},
				UseFFMPEG:     true,
				ServerAddress: "test.com:6078",
			},
			m3uContent: `#EXTM3U
#EXTINF:-1 tvg-id="id1" tvg-name="name1",Channel 1
http://example.com/channel1
#EXTINF:-1 tvg-id="id2" tvg-name="name2",Channel 2
http://example.com/channel2`,
			expectedM3u: `#EXTM3U
#EXTINF:-1 tvg-id="id1" tvg-name="name1",Channel 1
http://test.com:6078/channel/0
#EXTINF:-1 tvg-id="id2" tvg-name="name2",Channel 2
http://test.com:6078/channel/1
`,
			epgContent: `<?xml version="1.0" encoding="ISO-8859-1"?>
<!DOCTYPE tv SYSTEM "xmltv.dtd">
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file for m3u content
			tmpFile, err := createTempFile(tt.m3uContent, "test_m3u_*.m3u")
			if err != nil {
				t.Fatalf("Failed to create temporary file: %v", err)
			}
			defer os.Remove(tmpFile.Name())
			// Set the IPTVUrl to the temporary file path
			tt.config.IPTVUrl = filepath.ToSlash(tmpFile.Name())

			tmpFile, err = createTempFile(tt.epgContent, "test_epg_*.xml")
			if err != nil {
				t.Fatalf("Failed to create temporary file: %v", err)
			}
			defer os.Remove(tmpFile.Name())
			tt.config.EPGUrl = filepath.ToSlash(tmpFile.Name())

			tt.config.compileFilterRegexps()
			provider, err := NewProvider(tt.config)
			assert.NoError(t, err)

			err = provider.Refresh()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedM3u, provider.GetM3u())
			}
		})
	}
}

const refreshTestM3u = `#EXTM3U
#EXTINF:-1 tvg-id="id1" tvg-name="name1",Channel 1
http://example.com/channel1
`

const refreshTestEpg = `<?xml version="1.0" encoding="UTF-8"?>
<tv><channel id="id1"><display-name>Channel 1</display-name></channel></tv>
`

// newRefreshTestServer serves the playlist and EPG, with the EPG response
// controlled by epgStatus and epgBody so tests can make it fail.
func newRefreshTestServer(t *testing.T, epgStatus *int, epgBody *string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/iptv.m3u", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(refreshTestM3u))
	})
	mux.HandleFunc("/epg.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(*epgStatus)
		_, _ = w.Write([]byte(*epgBody))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func newRefreshTestProvider(t *testing.T, iptvURL, epgURL string) *Provider {
	config := &Config{
		IPTVUrl: iptvURL,
		EPGUrl:  epgURL,
		Filters: []*Filter{{Type: "id", Value: ".*"}},
	}
	assert.NoError(t, config.compileFilterRegexps())
	provider, err := NewProvider(config)
	assert.NoError(t, err)
	return provider
}

func TestProviderRefreshUnreachableURLReturnsError(t *testing.T) {
	// Grab a free address and close the listener so connections are refused.
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()

	provider := newRefreshTestProvider(t, url+"/iptv.m3u", url+"/epg.xml")

	assert.NotPanics(t, func() {
		assert.Error(t, provider.Refresh())
	})
}

func TestProviderRefreshFailureKeepsPreviousData(t *testing.T) {
	epgStatus := http.StatusOK
	epgBody := refreshTestEpg
	server := newRefreshTestServer(t, &epgStatus, &epgBody)

	provider := newRefreshTestProvider(t, server.URL+"/iptv.m3u", server.URL+"/epg.xml")
	assert.NoError(t, provider.Refresh())

	m3u := provider.GetM3u()
	epg := provider.GetEpgXML()
	lastRefresh := provider.GetLastRefresh()
	assert.Contains(t, m3u, "Channel 1")
	assert.Contains(t, epg, `<channel id="id1">`)

	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "non-200 EPG response", status: http.StatusInternalServerError, body: "oops"},
		{name: "truncated EPG", status: http.StatusOK, body: `<?xml version="1.0"?><tv><channel id="id1"><display-name>Chan`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			epgStatus = tt.status
			epgBody = tt.body

			assert.Error(t, provider.Refresh())
			assert.Equal(t, m3u, provider.GetM3u())
			assert.Equal(t, epg, provider.GetEpgXML())
			assert.Equal(t, lastRefresh, provider.GetLastRefresh())
		})
	}
}

func TestProviderGetTrackOutOfRange(t *testing.T) {
	provider := newRefreshTestProvider(t, "", "")
	assert.Same(t, &trackNotFound, provider.GetTrack(0))

	epgStatus := http.StatusOK
	epgBody := refreshTestEpg
	server := newRefreshTestServer(t, &epgStatus, &epgBody)
	provider = newRefreshTestProvider(t, server.URL+"/iptv.m3u", server.URL+"/epg.xml")
	assert.NoError(t, provider.Refresh())

	assert.Equal(t, "Channel 1", provider.GetTrack(0).Name)
	assert.Same(t, &trackNotFound, provider.GetTrack(1))
	assert.Same(t, &trackNotFound, provider.GetTrack(-1))
}

func TestProviderRefreshISO88591Epg(t *testing.T) {
	// "Télé" encoded as ISO-8859-1 (é = 0xE9).
	epgBody := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>\n" +
		"<tv><channel id=\"id1\"><display-name>T\xe9l\xe9</display-name></channel></tv>\n"
	epgStatus := http.StatusOK
	server := newRefreshTestServer(t, &epgStatus, &epgBody)

	provider := newRefreshTestProvider(t, server.URL+"/iptv.m3u", server.URL+"/epg.xml")
	assert.NoError(t, provider.Refresh())
	assert.Contains(t, provider.GetEpgXML(), "<display-name>Télé</display-name>")
}
