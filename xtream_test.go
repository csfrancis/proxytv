package proxytv

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newXtreamTestServer(t *testing.T, categories []xtreamCategory, streams []xtreamLiveStream) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "get_live_categories":
			assert.NoError(t, json.NewEncoder(w).Encode(categories))
		case "get_live_streams":
			assert.NoError(t, json.NewEncoder(w).Encode(streams))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestLoadXtreamPlaylist(t *testing.T) {
	categories := []xtreamCategory{
		{CategoryID: "1", CategoryName: "News"},
		{CategoryID: "2", CategoryName: "Sports"},
	}
	streams := []xtreamLiveStream{
		{Name: "Channel 1", StreamID: 100, CategoryID: "1", EPGChannelID: "chan1.tv", StreamIcon: "http://example.com/logo1.png"},
		{Name: "Channel 2", StreamID: 200, CategoryID: "2"},
	}

	server := newXtreamTestServer(t, categories, streams)
	defer server.Close()

	cfg := &XtreamConfig{
		Host:     server.URL,
		Username: "user",
		Password: "pass",
		Output:   "ts",
	}

	idFilter := &Filter{Type: "id", Value: ".*"}
	idFilter.regexp = regexp.MustCompile(idFilter.Value)
	pl := newPlaylistLoader("", []*Filter{idFilter})

	err := loadXtreamPlaylist(cfg, "", pl)
	assert.NoError(t, err)
	assert.Len(t, pl.tracks, 2)

	track1 := pl.tracks[0]
	assert.Equal(t, "Channel 1", track1.Name)
	assert.Equal(t, "chan1.tv", track1.Tags["tvg-id"])
	assert.Equal(t, "News", track1.Tags["group-title"])
	assert.Equal(t, "http://example.com/logo1.png", track1.Tags["tvg-logo"])
	assert.Equal(t, server.URL+"/live/user/pass/100.ts", track1.URI.String())

	track2 := pl.tracks[1]
	assert.Equal(t, "Channel 2", track2.Name)
	assert.Equal(t, "200", track2.Tags["tvg-id"], "falls back to stream id when epg_channel_id is empty")
	assert.Equal(t, "Sports", track2.Tags["group-title"])
	assert.Equal(t, server.URL+"/live/user/pass/200.ts", track2.URI.String())
}

func TestLoadXtreamPlaylistWithFilters(t *testing.T) {
	categories := []xtreamCategory{
		{CategoryID: "1", CategoryName: "News"},
		{CategoryID: "2", CategoryName: "Sports"},
	}
	streams := []xtreamLiveStream{
		{Name: "Channel 1", StreamID: 100, CategoryID: "1", EPGChannelID: "chan1.tv"},
		{Name: "Channel 2", StreamID: 200, CategoryID: "2", EPGChannelID: "chan2.tv"},
	}

	server := newXtreamTestServer(t, categories, streams)
	defer server.Close()

	cfg := &XtreamConfig{
		Host:     server.URL,
		Username: "user",
		Password: "pass",
		Output:   "ts",
	}

	filters := []*Filter{{Type: "group", Value: "^Sports$"}}
	filters[0].regexp = regexp.MustCompile(filters[0].Value)
	pl := newPlaylistLoader("", filters)

	err := loadXtreamPlaylist(cfg, "", pl)
	assert.NoError(t, err)

	assert.Len(t, pl.tracks, 1)
	assert.Equal(t, "Channel 2", pl.tracks[0].Name)
}

func TestLoadXtreamPlaylistEndToEnd(t *testing.T) {
	categories := []xtreamCategory{
		{CategoryID: "1", CategoryName: "News"},
	}
	streams := []xtreamLiveStream{
		{Name: "Channel 1", StreamID: 100, CategoryID: "1", EPGChannelID: "chan1.tv"},
	}

	server := newXtreamTestServer(t, categories, streams)
	defer server.Close()

	epgFile, err := createTempFile(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE tv SYSTEM "xmltv.dtd"><tv></tv>`, "epg_*.xml")
	assert.NoError(t, err)
	defer epgFile.Close()

	config := &Config{
		Xtream: &XtreamConfig{
			Host:     server.URL,
			Username: "user",
			Password: "pass",
			Output:   "ts",
		},
		EPGUrl: epgFile.Name(),
		Filters: []*Filter{
			{Type: "id", Value: ".*"},
		},
	}
	assert.NoError(t, config.compileFilterRegexps())

	provider, err := NewProvider(config)
	assert.NoError(t, err)

	assert.NoError(t, provider.Refresh())

	m3u := provider.GetM3u()
	assert.Contains(t, m3u, "Channel 1")
	assert.Contains(t, m3u, server.URL+"/live/user/pass/100.ts")
}
