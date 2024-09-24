package proxytv

import (
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
