package proxytv

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type Server struct {
	listenAddress string
	router        *gin.Engine
	server        *http.Server
	provider      *Provider
	useFfmpeg     bool
	maxStreams    int
	streamCount   int
}

func logrusLogFormatter(param gin.LogFormatterParams) string {
	log.WithFields(log.Fields{
		"clientIP":   param.ClientIP,
		"timeStamp":  param.TimeStamp.Format(time.RFC1123),
		"method":     param.Method,
		"path":       param.Path,
		"statusCode": param.StatusCode,
		"latency":    param.Latency,
	}).Debug("handled http request")

	return ""
}

func NewServer(config *Config, provider *Provider) (*Server, error) {
	server := &Server{
		listenAddress: config.ListenAddress,
		router:        gin.New(),
		provider:      provider,
		useFfmpeg:     config.UseFFMPEG,
		maxStreams:    config.MaxStreams,
	}

	server.router.Use(gin.LoggerWithFormatter(logrusLogFormatter))
	server.router.Use(gin.Recovery())

	return server, nil
}

func (s *Server) getIptvM3u() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Disposition", "attachment; filename=tv_channels.m3u")
		c.Header("Content-Description", "File Transfer")
		c.Header("Cache-Control", "no-cache")
		c.Data(200, "application/octet-stream", []byte(s.provider.GetM3u()))
	}
}

func (s *Server) getEpgXml() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(200, "application/xml", []byte(s.provider.GetEpgXml()))
	}
}

func (s *Server) remuxStream(c *gin.Context, track *Track, channelId int) {
	if s.streamCount > s.maxStreams {
		log.WithFields(log.Fields{
			"channelId":   channelId,
			"streamCount": s.streamCount,
			"maxStreams":  s.maxStreams,
		}).Warn("max streams reached")
		c.String(429, "Too many requests")
		return
	}

	logger := log.WithFields(log.Fields{
		"url":       track.URI.String(),
		"channelId": channelId,
		"clientIP":  c.RemoteIP(),
	})
	logger.Info("remuxing stream")

	start := time.Now()

	run := exec.Command("ffmpeg", "-i", track.URI.String(), "-c:v", "copy", "-f", "mpegts", "pipe:1")
	logger.WithField("cmd", strings.Join(run.Args, " ")).Debug("executing ffmpeg")
	ffmpegout, err := run.StdoutPipe()
	if err != nil {
		logger.WithError(err).Error("error creating ffmpeg stdout pipe")
		return
	}

	stderr, stderrErr := run.StderrPipe()
	if stderrErr != nil {
		logger.WithError(stderrErr).Errorln("error creating ffmpeg stderr pipe")
	}

	if startErr := run.Start(); startErr != nil {
		log.WithError(startErr).Errorln("error starting ffmpeg")
		return
	}
	defer run.Wait()

	s.streamCount++
	defer func() { s.streamCount-- }()

	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Split(split)
		for scanner.Scan() {
			log.Debugln(scanner.Text())
		}
	}()

	continueStream := true
	c.Header("Content-Type", `video/mpeg; codecs="avc1.4D401E"`)

	c.Stream(func(w io.Writer) bool {
		defer func() {
			logger.WithField("duration", time.Since(start)).Info("stopped streaming")
			if killErr := run.Process.Kill(); killErr != nil {
				logger.WithError(killErr).Error("error killing ffmpeg")
			}

			continueStream = false
		}()

		if _, copyErr := io.Copy(w, ffmpegout); copyErr != nil && !errors.Is(copyErr, syscall.EPIPE) {
			logger.WithError(copyErr).Error("error when copying data")
			continueStream = false
			return false
		}

		return continueStream
	})
}

func split(data []byte, atEOF bool) (advance int, token []byte, spliterror error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		// We have a full newline-terminated line.
		return i + 1, data[0:i], nil
	}

	if i := bytes.IndexByte(data, '\r'); i >= 0 {
		// We have a cr terminated line
		return i + 1, data[0:i], nil
	}

	if atEOF {
		return len(data), data, nil
	}

	return 0, nil, nil
}

func (s *Server) refresh() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Info("refreshing provider")
		if err := s.provider.Refresh(); err != nil {
			log.WithError(err).Error("error refreshing provider")
			c.String(500, "Error refreshing provider")
			return
		}
		c.String(200, "OK")
	}
}

func (s *Server) streamChannel() gin.HandlerFunc {
	return func(c *gin.Context) {
		channelIdParam := c.Param("channelId")
		channelId, err := strconv.Atoi(channelIdParam)
		if err != nil {
			log.WithError(err).Warn("invalid channelId")
			c.String(400, "Invalid channel id")
			return
		}

		if !s.useFfmpeg {
			c.String(404, "Channel not found")
			return
		}

		track := s.provider.GetTrack(channelId)
		if track.URI == nil {
			log.WithField("channelId", channelId).Warn("channel not found")
			c.String(404, "Channel not found")
			return
		}

		s.remuxStream(c, track, channelId)
	}
}

func (s *Server) Start(p *Provider) chan error {
	s.router.GET("/ping", func(c *gin.Context) {
		c.String(200, "PONG")
	})

	s.router.GET("/iptv.m3u", s.getIptvM3u())
	s.router.GET("/epg.xml", s.getEpgXml())
	s.router.GET("/channel/:channelId", s.streamChannel())
	s.router.PUT("/refresh", s.refresh())

	s.server = &http.Server{
		Addr:    s.listenAddress,
		Handler: s.router,
	}

	log.WithField("listenAddress", s.listenAddress).Info("starting http server")

	errChan := make(chan error, 1)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Error("failed to listen and serve")
			errChan <- err
		}
	}()

	return errChan
}

func (s *Server) Stop() error {
	// Create a context with a timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Info("stopping http server")

	// Shutdown server
	if err := s.server.Shutdown(ctx); err != nil {
		log.WithError(err).Error("server shutdown failed")
	}

	log.Info("http server stopped")

	return nil
}
