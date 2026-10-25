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
		{Name: "Channel 1", StreamID: "100", CategoryID: "1", EPGChannelID: "chan1.tv", StreamIcon: "http://example.com/logo1.png"},
		{Name: "Channel 2", StreamID: "200", CategoryID: "2"},
	}

	server := newXtreamTestServer(t, categories, streams)
	defer server.Close()

	cfg := &XtreamConfig{
		Host:     server.URL,
		Username: "user",
		Password: "pass",
		Output:   "ts",
	}

	groupFilter := &Filter{Type: "group", Value: ".*"}
	groupFilter.regexp = regexp.MustCompile(groupFilter.Value)
	pl := newPlaylistLoader("", []*Filter{groupFilter})

	err := loadXtreamPlaylist(cfg, "", pl)
	assert.NoError(t, err)
	assert.Len(t, pl.tracks, 2)

	track1 := pl.tracks[0]
	assert.Equal(t, "Channel 1", track1.Name)
	assert.Equal(t, "chan1.tv", track1.Tags["tvg-id"])
	assert.Equal(t, "News", track1.Tags["group-title"])
	assert.Equal(t, "http://example.com/logo1.png", track1.Tags["tvg-logo"])
	assert.Equal(t, server.URL+"/live/user/pass/100.ts", track1.URI.String())
	assert.Contains(t, track1.Raw, `tvg-logo="http://example.com/logo1.png"`)

	track2 := pl.tracks[1]
	assert.Equal(t, "Channel 2", track2.Name)
	assert.Equal(t, "", track2.Tags["tvg-id"], "empty epg_channel_id stays empty, like get.php")
	assert.Equal(t, "Sports", track2.Tags["group-title"])
	assert.Equal(t, server.URL+"/live/user/pass/200.ts", track2.URI.String())
}

func TestLoadXtreamPlaylistWithFilters(t *testing.T) {
	categories := []xtreamCategory{
		{CategoryID: "1", CategoryName: "News"},
		{CategoryID: "2", CategoryName: "Sports"},
	}
	streams := []xtreamLiveStream{
		{Name: "Channel 1", StreamID: "100", CategoryID: "1", EPGChannelID: "chan1.tv"},
		{Name: "Channel 2", StreamID: "200", CategoryID: "2", EPGChannelID: "chan2.tv"},
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
		{Name: "Channel 1", StreamID: "100", CategoryID: "1", EPGChannelID: "chan1.tv"},
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

func TestLoadXtreamPlaylistNumericAndStringIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "get_live_categories":
			_, _ = w.Write([]byte(`[{"category_id":1,"category_name":"News"},{"category_id":"2","category_name":"Sports"}]`))
		case "get_live_streams":
			_, _ = w.Write([]byte(`[{"name":"A","stream_id":"100","category_id":1},{"name":"B","stream_id":200,"category_id":"2"},{"name":"C","stream_id":null}]`))
		}
	}))
	defer server.Close()

	f := &Filter{Type: "group", Value: ".*"}
	f.regexp = regexp.MustCompile(f.Value)
	pl := newPlaylistLoader("", []*Filter{f})
	err := loadXtreamPlaylist(&XtreamConfig{Host: server.URL, Username: "u", Password: "p", Output: "ts"}, "", pl)
	assert.NoError(t, err)
	assert.Len(t, pl.tracks, 2, "stream without stream_id is skipped")
	assert.Equal(t, "News", pl.tracks[0].Tags["group-title"])
	assert.Equal(t, server.URL+"/live/u/p/100.ts", pl.tracks[0].URI.String())
	assert.Equal(t, "Sports", pl.tracks[1].Tags["group-title"])
	assert.Equal(t, server.URL+"/live/u/p/200.ts", pl.tracks[1].URI.String())
}

func TestLoadXtreamPlaylistEscapesCredentials(t *testing.T) {
	var gotUser, gotPass string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass = r.URL.Query().Get("username"), r.URL.Query().Get("password")
		switch r.URL.Query().Get("action") {
		case "get_live_categories":
			_, _ = w.Write([]byte(`[]`))
		case "get_live_streams":
			_, _ = w.Write([]byte(`[{"name":"A \"quoted\"","stream_id":7}]`))
		}
	}))
	defer server.Close()

	f := &Filter{Type: "name", Value: ".*"}
	f.regexp = regexp.MustCompile(f.Value)
	pl := newPlaylistLoader("", []*Filter{f})
	cfg := &XtreamConfig{Host: server.URL, Username: "me@x", Password: "p&ss/w#rd?", Output: "ts"}
	assert.NoError(t, loadXtreamPlaylist(cfg, "", pl))
	assert.Equal(t, "me@x", gotUser)
	assert.Equal(t, "p&ss/w#rd?", gotPass)
	assert.Len(t, pl.tracks, 1)
	assert.Equal(t, server.URL+"/live/me@x/p&ss%2Fw%23rd%3F/7.ts", pl.tracks[0].URI.String())
	assert.Contains(t, pl.tracks[0].Raw, `tvg-name="A 'quoted'"`)
}

func TestFetchXtreamJSONRedactsCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := server.URL
	server.Close() // connection refused from now on

	err := loadXtreamPlaylist(&XtreamConfig{Host: addr, Username: "secretuser", Password: "secretpass", Output: "ts"}, "", newPlaylistLoader("", nil))
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "secretuser")
	assert.NotContains(t, err.Error(), "secretpass")
}

func TestXtreamClientHasTimeout(t *testing.T) {
	assert.Equal(t, xtreamTimeout, xtreamClient.Timeout)
	assert.NotZero(t, xtreamClient.Timeout)
}
