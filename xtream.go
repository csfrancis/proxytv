package proxytv

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

type xtreamCategory struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
}

type xtreamLiveStream struct {
	Name         string `json:"name"`
	StreamID     int    `json:"stream_id"`
	StreamIcon   string `json:"stream_icon"`
	EPGChannelID string `json:"epg_channel_id"`
	CategoryID   string `json:"category_id"`
}

func fetchXtreamJSON(url string, userAgent string, target any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid url response code: %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

// loadXtreamPlaylist fetches the live channel list from an Xtream Codes
// panel's JSON API (player_api.php) instead of a plain M3U file, and feeds
// the result into handler as if it were a parsed M3U playlist. This is
// useful for panels that disable the bulk M3U export (get.php) while
// leaving the app JSON API open.
func loadXtreamPlaylist(cfg *XtreamConfig, userAgent string, handler m3uHandler) error {
	host := strings.TrimRight(cfg.Host, "/")

	var categories []xtreamCategory
	categoriesURL := fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=get_live_categories", host, cfg.Username, cfg.Password)
	if err := fetchXtreamJSON(categoriesURL, userAgent, &categories); err != nil {
		return fmt.Errorf("failed to fetch xtream categories: %w", err)
	}

	categoryNames := make(map[string]string, len(categories))
	for _, category := range categories {
		categoryNames[category.CategoryID] = category.CategoryName
	}

	var streams []xtreamLiveStream
	streamsURL := fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=get_live_streams", host, cfg.Username, cfg.Password)
	if err := fetchXtreamJSON(streamsURL, userAgent, &streams); err != nil {
		return fmt.Errorf("failed to fetch xtream live streams: %w", err)
	}

	handler.OnPlaylistStart()

	for _, stream := range streams {
		tvgID := stream.EPGChannelID
		if tvgID == "" {
			tvgID = strconv.Itoa(stream.StreamID)
		}

		tags := map[string]string{
			"tvg-id":   tvgID,
			"tvg-name": stream.Name,
		}
		if groupName, ok := categoryNames[stream.CategoryID]; ok {
			tags["group-title"] = groupName
		}
		if stream.StreamIcon != "" {
			tags["tvg-logo"] = stream.StreamIcon
		}

		uri := fmt.Sprintf("%s/live/%s/%s/%d.%s", host, cfg.Username, cfg.Password, stream.StreamID, cfg.Output)
		parsedURI, err := url.Parse(uri)
		if err != nil {
			log.WithError(err).WithField("uri", uri).Warn("skipping xtream stream with invalid uri")
			continue
		}

		track := &Track{
			Name: stream.Name,
			Tags: tags,
			URI:  parsedURI,
			Raw:  fmt.Sprintf(`#EXTINF:-1 tvg-id="%s" tvg-name="%s" tvg-logo="%s" group-title="%s",%s`, tags["tvg-id"], tags["tvg-name"], tags["tvg-logo"], tags["group-title"], stream.Name),
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
