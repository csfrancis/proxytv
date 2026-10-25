package proxytv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// xtreamTimeout bounds each player_api.php request. get_live_streams on a large
// panel is ~10 MB, so this is generous, but it stops a stalled panel from
// hanging Refresh (and therefore startup) forever.
const xtreamTimeout = 2 * time.Minute

var xtreamClient = &http.Client{Timeout: xtreamTimeout}

// flexString accepts a JSON string or number. Xtream panels disagree on whether
// IDs such as category_id and stream_id are strings or numbers.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*f = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

type xtreamCategory struct {
	CategoryID   flexString `json:"category_id"`
	CategoryName string     `json:"category_name"`
}

type xtreamLiveStream struct {
	Name         string     `json:"name"`
	StreamID     flexString `json:"stream_id"`
	StreamIcon   string     `json:"stream_icon"`
	EPGChannelID string     `json:"epg_channel_id"`
	CategoryID   flexString `json:"category_id"`
}

// redactURLError drops the request URL (which carries the account's username
// and password) from HTTP client errors before they are returned and logged.
func redactURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%s request failed: %w", urlErr.Op, urlErr.Err)
	}
	return err
}

func xtreamAPIURL(host string, cfg *XtreamConfig, action string) string {
	return fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=%s", host,
		url.QueryEscape(cfg.Username), url.QueryEscape(cfg.Password), url.QueryEscape(action))
}

func fetchXtreamJSON(url string, userAgent string, target any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return redactURLError(err)
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := xtreamClient.Do(req)
	if err != nil {
		return redactURLError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid url response code: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return redactURLError(err)
	}
	return nil
}

// m3uAttr makes a value safe inside a double-quoted M3U attribute.
func m3uAttr(s string) string {
	return strings.NewReplacer(`"`, `'`, "\n", " ", "\r", " ").Replace(s)
}

// loadXtreamPlaylist fetches the live channel list from an Xtream Codes
// panel's JSON API (player_api.php) instead of a plain M3U file, and feeds
// the result into handler as if it were a parsed M3U playlist. This is
// useful for panels that disable the bulk M3U export (get.php) while
// leaving the app JSON API open.
func loadXtreamPlaylist(cfg *XtreamConfig, userAgent string, handler m3uHandler) error {
	host := strings.TrimRight(cfg.Host, "/")

	var categories []xtreamCategory
	if err := fetchXtreamJSON(xtreamAPIURL(host, cfg, "get_live_categories"), userAgent, &categories); err != nil {
		return fmt.Errorf("failed to fetch xtream categories: %w", err)
	}

	categoryNames := make(map[string]string, len(categories))
	for _, category := range categories {
		categoryNames[string(category.CategoryID)] = category.CategoryName
	}

	var streams []xtreamLiveStream
	if err := fetchXtreamJSON(xtreamAPIURL(host, cfg, "get_live_streams"), userAgent, &streams); err != nil {
		return fmt.Errorf("failed to fetch xtream live streams: %w", err)
	}

	handler.OnPlaylistStart()

	for _, stream := range streams {
		if stream.StreamID == "" {
			log.WithField("name", stream.Name).Warn("skipping xtream stream without stream_id")
			continue
		}

		// Leave tvg-id empty when the panel has no EPG id, exactly like the
		// panel's own get.php export does, so "id" filters select the same
		// channels in xtream mode as in M3U mode.
		tags := map[string]string{
			"tvg-id":   stream.EPGChannelID,
			"tvg-name": stream.Name,
		}
		if groupName, ok := categoryNames[string(stream.CategoryID)]; ok {
			tags["group-title"] = groupName
		}
		if stream.StreamIcon != "" {
			tags["tvg-logo"] = stream.StreamIcon
		}

		uri := fmt.Sprintf("%s/live/%s/%s/%s.%s", host,
			url.PathEscape(cfg.Username), url.PathEscape(cfg.Password),
			url.PathEscape(string(stream.StreamID)), cfg.Output)
		parsedURI, err := url.Parse(uri)
		if err != nil {
			log.WithField("name", stream.Name).Warn("skipping xtream stream with invalid uri")
			continue
		}

		track := &Track{
			Name: stream.Name,
			Tags: tags,
			URI:  parsedURI,
			Raw: fmt.Sprintf(`#EXTINF:-1 tvg-id="%s" tvg-name="%s" tvg-logo="%s" group-title="%s",%s`,
				m3uAttr(tags["tvg-id"]), m3uAttr(tags["tvg-name"]), m3uAttr(tags["tvg-logo"]),
				m3uAttr(tags["group-title"]), strings.TrimSpace(strings.ReplaceAll(stream.Name, "\n", " "))),
		}
		handler.OnTrack(track)
	}

	handler.OnPlaylistEnd()

	log.WithFields(log.Fields{
		"channelCount":  len(streams),
		"categoryCount": len(categories),
	}).Info("loaded xtream playlist")

	return nil
}
