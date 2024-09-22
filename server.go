package proxytv

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type Server struct {
	listenAddress string
	router        *gin.Engine
	server        *http.Server
	provider      *Provider
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

func (s *Server) Start(p *Provider) {
	s.router.GET("/ping", func(c *gin.Context) {
		c.String(200, "PONG")
	})

	s.router.GET("/get.php", s.getIptvM3u())
	s.router.GET("/xmltv.php", s.getEpgXml())

	s.server = &http.Server{
		Addr:    s.listenAddress,
		Handler: s.router,
	}

	log.WithField("listenAddress", s.listenAddress).Info("starting http server")

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Error("failed to listen and serve")
		}
	}()
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
