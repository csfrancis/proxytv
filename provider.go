package proxytv

import (
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/csfrancis/proxytv/xmltv"
	"golang.org/x/net/html/charset"

	log "github.com/sirupsen/logrus"
)

type playlistLoader struct {
	baseAddress string
	filters     []*Filter

	tracks     []Track
	priorities map[string]int
	m3u        strings.Builder
}

func newPlaylistLoader(baseAddress string, filters []*Filter) *playlistLoader {
	return &playlistLoader{
		baseAddress: baseAddress,
		filters:     filters,
		tracks:      make([]Track, 0, len(filters)),
		priorities:  make(map[string]int),
	}
}

func (pl *playlistLoader) findIndexWithID(track *Track) int {
	id := track.Tags["tvg-id"]
	if len(id) == 0 {
		return -1
	}

	for i := range pl.tracks {
		if pl.tracks[i].Tags["tvg-id"] == id {
			return i
		}
	}
	return -1
}

func (pl *playlistLoader) OnPlaylistStart() {
	pl.m3u.Reset()
	pl.m3u.WriteString("#EXTM3U\n")
}

func (pl *playlistLoader) OnTrack(track *Track) {
	for i, filter := range pl.filters {
		var field string
		switch filter.Type {
		case "id":
			field = "tvg-id"
		case "group":
			field = "group-title"
		case "name":
			field = "tvg-name"
		default:
			log.WithField("type", filter.Type).Panic("invalid filter type")
		}

		val := track.Tags[field]
		if len(val) == 0 {
			continue
		}

		if filter.regexp.Match([]byte(val)) {
			name := track.Name

			if len(track.Tags["tvg-id"]) == 0 {
				log.WithField("track", track).Warn("missing tvg-id")
			}

			if existingPriority, exists := pl.priorities[name]; !exists || i < existingPriority {
				idx := pl.findIndexWithID(track)
				if idx != -1 {
					if strings.Contains(track.Name, "HD") {
						delete(pl.priorities, pl.tracks[idx].Name)
						pl.tracks[idx] = *track
					} else {
						continue
					}
				} else {
					if !exists {
						pl.tracks = append(pl.tracks, *track)
					}
				}
				pl.priorities[name] = i
			} else if exists {
				log.WithField("track", track).Warn("duplicate name")
			}
		}
	}
}

func (pl *playlistLoader) OnPlaylistEnd() {
	sort.SliceStable(pl.tracks, func(i, j int) bool {
		priorityI, existsI := pl.priorities[pl.tracks[i].Name]
		priorityJ, existsJ := pl.priorities[pl.tracks[j].Name]

		if !existsI && !existsJ {
			return false // Keep original order for unmatched elements
		}
		if !existsI {
			return false // Unmatched elements go to the end
		}
		if !existsJ {
			return true // Matched elements come before unmatched ones
		}
		return priorityI < priorityJ
	})

	rewriteURL := len(pl.baseAddress) > 0

	reXuiid := regexp.MustCompile(`xui-id="\{[^"]*\}"\s*`)

	for i := range len(pl.tracks) {
		track := pl.tracks[i]
		uri := track.URI.String()
		if rewriteURL {
			uri = fmt.Sprintf("http://%s/channel/%d", pl.baseAddress, i)
		}
		// Remove xui-id from the tags
		fixedRaw := reXuiid.ReplaceAllString(track.Raw, "")
		pl.m3u.WriteString(fmt.Sprintf("%s\n%s\n", fixedRaw, uri))
	}
}

// httpClient is used to fetch the IPTV playlist and EPG. The transport-level
// timeouts make an unresponsive provider fail fast, while the overall timeout is
// generous enough to allow large EPG downloads but still bounds a stalled body.
var httpClient = &http.Client{
	Timeout: 10 * time.Minute,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 2 * time.Minute,
		IdleConnTimeout:       90 * time.Second,
	},
}

func loadReader(uri string, userAgent string) (io.ReadCloser, error) {
	if !isURL(uri) {
		return os.Open(uri)
	}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to load uri: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("invalid url response code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

type Provider struct {
	iptvURL     string
	epgURL      string
	baseAddress string
	userAgent   string
	filters     []*Filter

	// lock guards the fields below, which are replaced as a unit by Refresh
	// and read concurrently by the HTTP server.
	lock        sync.RWMutex
	playlist    *playlistLoader
	epg         *xmltv.TV
	epgData     []byte
	lastRefresh time.Time
}

func NewProvider(config *Config) (*Provider, error) {
	provider := &Provider{
		iptvURL: config.IPTVUrl,
		epgURL:  config.EPGUrl,
		filters: config.Filters,
	}

	if len(config.UserAgent) > 0 {
		provider.userAgent = config.UserAgent
	}

	if config.UseFFMPEG {
		provider.baseAddress = config.ServerAddress
	}

	return provider, nil
}

func loadXMLTv(reader io.Reader, tracks []Track) (*xmltv.TV, error) {
	start := time.Now()

	channels := make(map[string]bool)
	for _, track := range tracks {
		id := track.Tags["tvg-id"]
		if len(id) == 0 {
			continue
		}
		channels[id] = true
	}

	decoder := xml.NewDecoder(reader)
	// XMLTV feeds commonly declare non-UTF-8 encodings such as ISO-8859-1.
	decoder.CharsetReader = charset.NewReaderLabel
	tvSetup := new(xmltv.TV)

	totalChannelCount := 0
	totalProgrammeCount := 0

	for {
		// Decode the next XML token
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Don't treat a truncated or corrupt EPG (e.g. a dropped connection
			// mid-download) as a successful load.
			return nil, err
		}

		// Process the start element
		switch se := tok.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "tv":
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "date":
						tvSetup.Date = attr.Value
					case "source-info-url":
						tvSetup.SourceInfoURL = attr.Value
					case "source-info-name":
						tvSetup.SourceInfoName = attr.Value
					case "source-data-url":
						tvSetup.SourceDataURL = attr.Value
					case "generator-info-name":
						tvSetup.GeneratorInfoName = attr.Value
					case "generator-info-url":
						tvSetup.GeneratorInfoURL = attr.Value
					}
				}
			case "programme":
				var programme xmltv.Programme
				err := decoder.DecodeElement(&programme, &se)
				if err != nil {
					return nil, err
				}
				if channels[programme.Channel] {
					tvSetup.Programmes = append(tvSetup.Programmes, programme)
				}
				totalProgrammeCount++
			case "channel":
				var channel xmltv.Channel
				err := decoder.DecodeElement(&channel, &se)
				if err != nil {
					return nil, err
				}
				if channels[channel.ID] {
					tvSetup.Channels = append(tvSetup.Channels, channel)
				}
				totalChannelCount++
			}
		}
	}

	log.WithFields(log.Fields{
		"totalChannelCount":   totalChannelCount,
		"channelCount":        len(tvSetup.Channels),
		"totalProgrammeCount": totalProgrammeCount,
		"programmeCount":      len(tvSetup.Programmes),
		"duration":            time.Since(start),
	}).Info("loaded xmltv")

	return tvSetup, nil
}

// Refresh reloads the playlist and EPG. Everything is loaded before any state is
// replaced, so if any step fails the previously loaded data continues to be served.
func (p *Provider) Refresh() error {
	log.WithField("url", p.iptvURL).Info("loading IPTV m3u")

	start := time.Now()
	iptvReader, err := loadReader(p.iptvURL, p.userAgent)
	if err != nil {
		return err
	}
	defer iptvReader.Close()
	log.WithField("duration", time.Since(start)).Debug("loaded IPTV m3u")

	pl := newPlaylistLoader(p.baseAddress, p.filters)
	if err := loadM3u(iptvReader, pl); err != nil {
		return err
	}

	log.WithField("channelCount", len(pl.tracks)).Info("parsed IPTV m3u")

	log.WithField("url", p.epgURL).Info("loading EPG")

	start = time.Now()
	epgReader, err := loadReader(p.epgURL, p.userAgent)
	if err != nil {
		return err
	}
	defer epgReader.Close()
	log.WithField("duration", time.Since(start)).Debug("loaded EPG")

	epg, err := loadXMLTv(epgReader, pl.tracks)
	if err != nil {
		return err
	}

	xmlData, err := xml.Marshal(epg)
	if err != nil {
		return err
	}

	xmlHeader := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?><!DOCTYPE tv SYSTEM \"xmltv.dtd\">")
	epgData := append(xmlHeader, xmlData...)

	p.lock.Lock()
	p.playlist = pl
	p.epg = epg
	p.epgData = epgData
	p.lastRefresh = time.Now()
	p.lock.Unlock()

	return nil
}

func (p *Provider) GetM3u() string {
	p.lock.RLock()
	defer p.lock.RUnlock()
	if p.playlist == nil {
		return ""
	}
	return p.playlist.m3u.String()
}

func (p *Provider) GetEpgXML() string {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return string(p.epgData)
}

var trackNotFound = Track{}

func (p *Provider) GetTrack(idx int) *Track {
	p.lock.RLock()
	defer p.lock.RUnlock()
	if p.playlist == nil || idx < 0 || idx >= len(p.playlist.tracks) {
		return &trackNotFound
	}
	// The returned pointer stays valid after a refresh: Refresh swaps in a new
	// playlist rather than modifying the existing tracks slice.
	return &p.playlist.tracks[idx]
}

func (p *Provider) GetLastRefresh() time.Time {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.lastRefresh
}
