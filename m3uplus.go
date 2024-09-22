package proxytv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type m3uHandler interface {
	OnPlaylistStart()
	OnTrack(track track)
	OnPlaylistEnd()
}

type track struct {
	Name       string
	Length     float64
	URI        *url.URL
	Tags       map[string]string
	Raw        string
	LineNumber int
}

var errMalformedM3U = errors.New("malformed M3U provided")
var errMissingExtinf = errors.New("URL found without preceding EXTINF")

func decodeM3u(r io.Reader, handler m3uHandler) error {
	scanner := bufio.NewScanner(r)
	lineNum := 0
	var currentTrack *track

	handler.OnPlaylistStart()

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if lineNum == 1 && !strings.HasPrefix(line, "#EXTM3U") {
			return errMalformedM3U
		}

		switch {
		case strings.HasPrefix(line, "#EXTINF:"):
			if currentTrack != nil {
				handler.OnTrack(*currentTrack)
			}
			currentTrack = &track{
				Raw:        line,
				LineNumber: lineNum,
			}
			var err error
			currentTrack.Length, currentTrack.Name, currentTrack.Tags, err = decodeInfoLine(line)
			if err != nil {
				return err
			}

		case isUrl(line):
			if currentTrack == nil {
				return errMissingExtinf
			}
			uri, _ := url.Parse(line)
			currentTrack.URI = uri
			handler.OnTrack(*currentTrack)
			currentTrack = nil
		}
	}

	if currentTrack != nil {
		handler.OnTrack(*currentTrack)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	handler.OnPlaylistEnd()

	return nil
}

func isUrl(str string) bool {
	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

var infoRegex = regexp.MustCompile(`([^\s="]+)=(?:"(.*?)"|(\d+))(?:,([.*^,]))?|#EXTINF:(-?\d*\s*)|,(.*)`)

func decodeInfoLine(line string) (float64, string, map[string]string, error) {
	matches := infoRegex.FindAllStringSubmatch(line, -1)
	var err error
	durationFloat := 0.0
	durationStr := strings.TrimSpace(matches[0][len(matches[0])-2])
	if durationStr != "-1" && len(durationStr) > 0 {
		if durationFloat, err = strconv.ParseFloat(durationStr, 64); err != nil {
			return 0, "", nil, fmt.Errorf("duration parsing error: %s", err)
		}
	}

	titleIndex := len(matches) - 1
	title := matches[titleIndex][len(matches[titleIndex])-1]

	keyMap := make(map[string]string)

	for _, match := range matches[1 : len(matches)-1] {
		val := match[2]
		if val == "" {
			val = match[3]
		}
		keyMap[strings.ToLower(match[1])] = val
	}

	return durationFloat, title, keyMap, nil
}
