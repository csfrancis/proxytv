package proxytv

import (
	"io"
	"net/http"
	"os"

	log "github.com/sirupsen/logrus"
)

type Provider struct {
	IPTVUrl string
	EPGUrl  string
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

func (p *Provider) OnPlaylistStart() {

}

func (p *Provider) OnTrack(track track) {

}

func (p *Provider) OnPlaylistEnd() {

}

func NewProvider(config *Config) (*Provider, error) {
	return &Provider{
		IPTVUrl: config.IPTVUrl,
		EPGUrl:  config.EPGUrl,
	}, nil
}

func (p *Provider) Load() error {
	iptvReader := loadReader(p.IPTVUrl)
	err := decodeM3u(iptvReader, p)
	if err != nil {
		return err
	}

	return nil
}
