package proxytv

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockHandler struct {
	playlistStartCalled bool
	tracks              []track
	playlistEndCalled   bool
}

func (m *mockHandler) OnPlaylistStart() {
	m.playlistStartCalled = true
}

func (m *mockHandler) OnTrack(t track) {
	m.tracks = append(m.tracks, t)
}

func (m *mockHandler) OnPlaylistEnd() {
	m.playlistEndCalled = true
}

func TestDecodeM3u(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected mockHandler
		wantErr  bool
	}{
		{
			name: "Valid M3U",
			input: `#EXTM3U
#EXTINF:-1 tvg-id="id1" tvg-name="name1",Channel 1
http://example.com/channel1
#EXTINF:-1 tvg-id="id2" tvg-name="name2",Channel 2
http://example.com/channel2`,
			expected: mockHandler{
				playlistStartCalled: true,
				tracks: []track{
					{
						Name:   "Channel 1",
						Length: 0,
						URI:    mustParseURL("http://example.com/channel1"),
						Tags:   map[string]string{"tvg-id": "id1", "tvg-name": "name1"},
					},
					{
						Name:   "Channel 2",
						Length: 0,
						URI:    mustParseURL("http://example.com/channel2"),
						Tags:   map[string]string{"tvg-id": "id2", "tvg-name": "name2"},
					},
				},
				playlistEndCalled: true,
			},
			wantErr: false,
		},
		{
			name:     "Invalid M3U (missing #EXTM3U)",
			input:    "Invalid content",
			expected: mockHandler{},
			wantErr:  true,
		},
		{
			name: "Missing EXTINF",
			input: `#EXTM3U
http://example.com/channel1`,
			expected: mockHandler{
				playlistStartCalled: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &mockHandler{}
			err := decodeM3u(strings.NewReader(tt.input), handler)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.playlistStartCalled, handler.playlistStartCalled)
				assert.Equal(t, tt.expected.playlistEndCalled, handler.playlistEndCalled)
				assert.Equal(t, len(tt.expected.tracks), len(handler.tracks))

				for i, expectedTrack := range tt.expected.tracks {
					assert.Equal(t, expectedTrack.Name, handler.tracks[i].Name)
					assert.Equal(t, expectedTrack.Length, handler.tracks[i].Length)
					assert.Equal(t, expectedTrack.URI, handler.tracks[i].URI)
					assert.Equal(t, expectedTrack.Tags, handler.tracks[i].Tags)
				}
			}
		})
	}
}

func mustParseURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}
