package proxytv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProviderLoad(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		m3uContent  string
		expectedM3u string
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
			wantErr: false,
		},
		{
			name: "Filtered valid M3U content",
			config: &Config{
				Filters: []*Filter{
					{Type: "id", Value: ".*"},
				},
				UseFFMPEG:   true,
				BaseAddress: "test.com:6078",
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
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file
			tmpFile, err := os.CreateTemp("", "test_m3u_*.m3u")
			assert.NoError(t, err)
			defer os.Remove(tmpFile.Name())

			// Write the M3U content to the temporary file
			_, err = tmpFile.WriteString(tt.m3uContent)
			assert.NoError(t, err)
			tmpFile.Close()

			// Set the IPTVUrl to the temporary file path
			tt.config.IPTVUrl = filepath.ToSlash(tmpFile.Name())

			tt.config.compileFilterRegexps()
			provider, err := NewProvider(tt.config)
			assert.NoError(t, err)

			err = provider.Load()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedM3u, provider.GetM3u())
			}
		})
	}
}
