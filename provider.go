package proxytv

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/csfrancis/proxytv/xmltv"

	log "github.com/sirupsen/logrus"
)

type Provider struct {
	iptvUrl     string
	epgUrl      string
	baseAddress string
	filters     []*Filter

	tracks     []track
	priorities map[string]int
	m3uData    strings.Builder
	epg        *xmltv.TV
	epgData    []byte
}

func loadReader(uri string) io.ReadCloser {
	var err error
	var reader io.ReadCloser
	logger := log.WithField("uri", uri)
	if isUrl(uri) {
		resp, err := http.Get(uri)
		if err != nil {
			logger.WithError(err).Panic("unable to load uri")
		}

		if resp.StatusCode != http.StatusOK {
			logger.WithField("status", resp.StatusCode).Panic("invalid url response code")
		}

		reader = resp.Body
	} else {
		reader, err = os.Open(uri)
		if err != nil {
			logger.WithError(err).Panic("error loading file")
		}
	}

	return reader
}

func (p *Provider) findIndexWithId(track track) int {
	id := track.Tags["tvg-id"]
	if len(id) == 0 {
		return -1
	}

	for i := range p.tracks {
		if p.tracks[i].Tags["tvg-id"] == id {
			return i
		}
	}
	return -1
}

func (p *Provider) OnPlaylistStart() {
	p.m3uData.Reset()
	p.m3uData.WriteString("#EXTM3U\n")
}

func (p *Provider) OnTrack(track track) {
	for i, filter := range p.filters {
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

			if existingPriority, exists := p.priorities[name]; !exists || i < existingPriority {
				idx := p.findIndexWithId(track)
				if idx != -1 {
					if strings.Contains(track.Name, "HD") {
						delete(p.priorities, p.tracks[idx].Name)
						p.tracks[idx] = track
					} else {
						continue
					}
				} else {
					if !exists {
						p.tracks = append(p.tracks, track)
					}
				}
				p.priorities[name] = i
			} else if exists {
				log.WithField("track", track).Warn("duplicate name")
			}
		}
	}
}

func (p *Provider) OnPlaylistEnd() {
	sort.SliceStable(p.tracks, func(i, j int) bool {
		priorityI, existsI := p.priorities[p.tracks[i].Name]
		priorityJ, existsJ := p.priorities[p.tracks[j].Name]

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

	rewriteUrl := len(p.baseAddress) > 0

	for i := range len(p.tracks) {
		track := p.tracks[i]
		uri := track.URI.String()
		if rewriteUrl {
			uri = fmt.Sprintf("http://%s/channel/%d", p.baseAddress, i)
		}
		p.m3uData.WriteString(fmt.Sprintf("%s\n%s\n", track.Raw, uri))
	}
}

func NewProvider(config *Config) (*Provider, error) {
	provider := &Provider{
		iptvUrl:    config.IPTVUrl,
		epgUrl:     config.EPGUrl,
		filters:    config.Filters,
		tracks:     make([]track, 0, len(config.Filters)),
		priorities: make(map[string]int),
	}

	if config.UseFFMPEG {
		provider.baseAddress = config.BaseAddress
	}

	return provider, nil
}

func (p *Provider) loadXmlTv(reader io.Reader) (*xmltv.TV, error) {
	channels := make(map[string]bool)
	for _, track := range p.tracks {
		id := track.Tags["tvg-id"]
		if len(id) == 0 {
			continue
		}
		channels[id] = true
	}

	decoder := xml.NewDecoder(reader)
	tvSetup := new(xmltv.TV)

	for {
		// Decode the next XML token
		tok, err := decoder.Token()
		if err != nil {
			break // Exit on EOF or error
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
			case "channel":
				var channel xmltv.Channel
				err := decoder.DecodeElement(&channel, &se)
				if err != nil {
					return nil, err
				}
				if channels[channel.ID] {
					tvSetup.Channels = append(tvSetup.Channels, channel)
				}
			}
		}
	}

	log.WithField("channelCount", len(tvSetup.Channels)).WithField("programmeCount", len(tvSetup.Programmes)).Info("loaded xmltv")

	return tvSetup, nil
}

func (p *Provider) Refresh() error {
	var err error
	log.WithField("url", p.iptvUrl).Info("loading IPTV m3u")

	start := time.Now()
	iptvReader := loadReader(p.iptvUrl)
	defer iptvReader.Close()
	log.WithField("duration", time.Since(start)).Debug("loaded IPTV m3u")

	err = decodeM3u(iptvReader, p)
	if err != nil {
		return err
	}
	log.WithField("channelCount", len(p.tracks)).Info("parsed IPTV m3u")

	log.WithField("url", p.epgUrl).Info("loading EPG")

	start = time.Now()
	epgReader := loadReader(p.epgUrl)
	defer epgReader.Close()
	log.WithField("duration", time.Since(start)).Debug("loaded EPG")

	p.epg, err = p.loadXmlTv(epgReader)
	if err != nil {
		return err
	}

	log.WithField("channelCount", len(p.epg.Channels)).WithField("programmeCount", len(p.epg.Programmes)).Info("parsed EPG")

	xmlData, err := xml.Marshal(p.epg)
	if err != nil {
		return err
	}

	xmlHeader := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?><!DOCTYPE tv SYSTEM \"xmltv.dtd\">")
	p.epgData = append(xmlHeader, xmlData...)

	return nil
}

func (p *Provider) GetM3u() string {
	return p.m3uData.String()
}

func (p *Provider) GetEpgXml() string {
	return string(p.epgData)
}
