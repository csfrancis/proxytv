package proxytv

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"runtime"

	"sync/atomic"

	"html/template"

	"github.com/csfrancis/proxytv/data/static"
	"github.com/csfrancis/proxytv/data/templates"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

const channelURIPrefix = "/channel/"

// streamInfoKey is the gin context key holding the *streamInfo for a stream request.
const streamInfoKey = "streamInfo"

// errStreamDisconnected is the cancellation cause used when a stream is
// disconnected from the web UI.
var errStreamDisconnected = errors.New("stream disconnected from web ui")

var startTime = time.Now()

type Server struct {
	listenAddress string
	router        *gin.Engine
	server        *http.Server
	provider      *Provider
	useFfmpeg     bool
	streamsSem    *semaphore.Weighted
	maxStreams    int64
	totalStreams  int64
	nextStreamID  atomic.Uint64
	streams       map[uint64]*streamInfo
	lock          sync.Mutex
	version       string
	headContent   template.HTML
}

type streamInfo struct {
	ID        uint64    `json:"id"`
	ClientIP  string    `json:"clientIP"`
	ChannelID int       `json:"channelID"`
	Name      string    `json:"name,omitempty"`
	LogoURL   string    `json:"logoUrl,omitempty"`
	StartTime time.Time `json:"startTime"`

	// cancel ends the stream by cancelling its request context, which also
	// kills the ffmpeg process serving it.
	cancel context.CancelCauseFunc
}

func newStreamInfo(request *http.Request) (*streamInfo, error) {
	channelID, err := strconv.Atoi(request.RequestURI[len(channelURIPrefix):])
	if err != nil {
		return nil, err
	}

	return &streamInfo{
		ClientIP:  request.RemoteAddr,
		ChannelID: channelID,
		StartTime: time.Now(),
	}, nil
}

func logrusLogFormatter(param gin.LogFormatterParams) string {
	log.WithFields(log.Fields{
		"clientIP":   param.ClientIP,
		"method":     param.Method,
		"path":       param.Path,
		"statusCode": param.StatusCode,
		"latency":    param.Latency,
	}).Debug("handled http request")

	return ""
}

type fsAdapter struct {
	fs    http.FileSystem
	names []string
}

func (f *fsAdapter) Open(name string) (fs.File, error) {
	return f.fs.Open(name)
}

func (f *fsAdapter) Glob(pattern string) ([]string, error) {
	matches := []string{}
	for _, name := range f.names {
		if matched, err := path.Match(pattern, name); err != nil {
			return nil, err
		} else if matched {
			matches = append(matches, name)
		}
	}
	return matches, nil
}

func NewServer(config *Config, provider *Provider, version string) (*Server, error) {
	server := &Server{
		listenAddress: config.ListenAddress,
		router:        gin.New(),
		provider:      provider,
		useFfmpeg:     config.UseFFMPEG,
		streamsSem:    semaphore.NewWeighted(int64(config.MaxStreams)),
		maxStreams:    int64(config.MaxStreams),
		totalStreams:  0,
		streams:       make(map[uint64]*streamInfo),
		version:       version,
		headContent:   headContent(version),
	}

	server.router.Use(gin.LoggerWithFormatter(logrusLogFormatter))
	server.router.Use(gin.Recovery())

	// Load HTML templates
	fsa := &fsAdapter{fs: templates.AssetFile(), names: templates.AssetNames()}
	templ := template.Must(template.ParseFS(fsa, "*.html"))
	server.router.SetHTMLTemplate(templ)

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

func (s *Server) getEpgXML() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(200, "application/xml", []byte(s.provider.GetEpgXML()))
	}
}

func (s *Server) remuxStream(c *gin.Context, track *Track, channelID int) {
	reqCtx := c.Request.Context()
	ctx, cancel := context.WithTimeout(reqCtx, 3*time.Second)
	defer cancel()

	if err := s.streamsSem.Acquire(ctx, 1); err != nil {
		log.WithFields(log.Fields{
			"channelId": channelID,
		}).Warn("max streams reached")
		c.String(429, "Too many requests")
		return
	}
	defer s.streamsSem.Release(1)

	if value, ok := c.Get(streamInfoKey); ok {
		streamInfo := value.(*streamInfo)
		s.lock.Lock()
		streamInfo.Name = track.Name
		if logo, ok := track.Tags["tvg-logo"]; ok {
			streamInfo.LogoURL = logo
		}
		s.lock.Unlock()
	} else {
		log.Warn("no stream info found")
	}

	logger := log.WithFields(log.Fields{
		"url":       track.URI.String(),
		"channelId": channelID,
		"clientIP":  c.RemoteIP(),
	})
	logger.Info("remuxing stream")

	start := time.Now()

	// Use the request context so ffmpeg is killed if the stream is disconnected.
	run := exec.CommandContext(reqCtx, "ffmpeg", "-i", track.URI.String(), "-c:v", "copy", "-f", "mpegts", "pipe:1")
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

	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Split(split)
		for scanner.Scan() {
			log.Debugln(scanner.Text())
		}
	}()

	atomic.AddInt64(&s.totalStreams, 1)

	// When the stream is cancelled (e.g. disconnected from the web UI), fail any
	// pending write to the client immediately rather than waiting for buffered
	// data to drain, so the client connection is dropped right away.
	stopDeadline := context.AfterFunc(reqCtx, func() {
		if err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Now()); err != nil {
			logger.WithError(err).Warn("unable to set write deadline")
		}
	})
	defer stopDeadline()

	bytesWritten := int64(0)
	c.Header("Content-Type", `video/mpeg; codecs="avc1.4D401E"`)

	// The step function copies until ffmpeg exits or an error occurs, so the
	// stream is finished once it returns.
	c.Stream(func(w io.Writer) bool {
		defer func() {
			fields := log.Fields{
				"duration": time.Since(start),
				"bytes":    bytesWritten,
			}
			if errors.Is(context.Cause(reqCtx), errStreamDisconnected) {
				fields["reason"] = errStreamDisconnected.Error()
			}
			logger.WithFields(fields).Info("stopped streaming")
			if killErr := run.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				logger.WithError(killErr).Error("error killing ffmpeg")
			}
		}()

		timeoutReader := NewTimeoutReader(ffmpegout, 30*time.Second)
		timeoutWriter := NewTimeoutWriter(w, 30*time.Second)

		// Errors after the stream has been cancelled are expected and not logged.
		if bytesWritten, err = io.Copy(timeoutWriter, timeoutReader); err != nil && reqCtx.Err() == nil {
			if err == ErrTimeout {
				logger.Warn("timeout occurred during stream copy")
			} else if !errors.Is(err, syscall.EPIPE) {
				logger.WithError(err).Error("error when copying data")
			}
		}

		return false
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
		c.String(200, "Provider refreshed successfully")
	}
}

func (s *Server) streamChannel() gin.HandlerFunc {
	return func(c *gin.Context) {
		channelIDParam := c.Param("channelId")
		channelID, err := strconv.Atoi(channelIDParam)
		if err != nil {
			log.WithError(err).Warn("invalid channelId")
			c.String(400, "Invalid channel id")
			return
		}

		if !s.useFfmpeg {
			c.String(404, "Channel not found")
			return
		}

		track := s.provider.GetTrack(channelID)
		if track.URI == nil {
			log.WithField("channelId", channelID).Warn("channel not found")
			c.String(404, "Channel not found")
			return
		}

		s.remuxStream(c, track, channelID)
	}
}

func (s *Server) streamTracker(c *gin.Context) {
	if !strings.HasPrefix(c.Request.RequestURI, channelURIPrefix) {
		c.Next()
		return
	}

	streamInfo, err := newStreamInfo(c.Request)
	if err != nil {
		log.WithError(err).Error("error creating stream info")
		c.Next()
		return
	}

	ctx, cancel := context.WithCancelCause(c.Request.Context())
	defer cancel(nil)
	c.Request = c.Request.WithContext(ctx)

	streamInfo.ID = s.nextStreamID.Add(1)
	streamInfo.cancel = cancel
	c.Set(streamInfoKey, streamInfo)

	s.lock.Lock()
	s.streams[streamInfo.ID] = streamInfo
	s.lock.Unlock()

	defer func() {
		s.lock.Lock()
		delete(s.streams, streamInfo.ID)
		s.lock.Unlock()
	}()

	c.Next()
}

// getActiveStreams returns a snapshot of the active streams, oldest first.
func (s *Server) getActiveStreams() []streamInfo {
	s.lock.Lock()
	streams := make([]streamInfo, 0, len(s.streams))
	for _, stream := range s.streams {
		streams = append(streams, *stream)
	}
	s.lock.Unlock()

	sort.Slice(streams, func(i, j int) bool { return streams[i].ID < streams[j].ID })
	return streams
}

// disconnectStream ends an active stream and responds with the updated stream list.
func (s *Server) disconnectStream() gin.HandlerFunc {
	return func(c *gin.Context) {
		streamID, err := strconv.ParseUint(c.Param("streamId"), 10, 64)
		if err != nil {
			log.WithError(err).Warn("invalid streamId")
			c.String(http.StatusBadRequest, "Invalid stream id")
			return
		}

		// Remove the stream from the list immediately so the UI reflects the
		// disconnect; the stream's own handler finishes shutting down ffmpeg.
		s.lock.Lock()
		stream, ok := s.streams[streamID]
		if ok {
			delete(s.streams, streamID)
		}
		s.lock.Unlock()

		if ok {
			log.WithFields(log.Fields{
				"streamId":  streamID,
				"channelId": stream.ChannelID,
				"clientIP":  stream.ClientIP,
			}).Info("disconnecting stream")
			stream.cancel(errStreamDisconnected)
		} else {
			log.WithField("streamId", streamID).Warn("stream not found")
		}

		c.HTML(http.StatusOK, "stream_info.html", s.getStreamInfoData())
	}
}

func (s *Server) debug() gin.HandlerFunc {
	return func(c *gin.Context) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		numGoroutines := runtime.NumGoroutine()
		numCPU := runtime.NumCPU()
		activeStreams := s.getActiveStreams()
		totalStreams := atomic.LoadInt64(&s.totalStreams)

		metrics := gin.H{
			"status":  "ok",
			"version": s.version,
			"system": gin.H{
				"memory": gin.H{
					"alloc":      m.Alloc,
					"totalAlloc": m.TotalAlloc,
					"sys":        m.Sys,
					"numGC":      m.NumGC,
				},
				"goroutines": numGoroutines,
				"cpus":       numCPU,
			},
			"uptime": time.Since(startTime).String(),
			"streams": gin.H{
				"active":      activeStreams,
				"max":         s.maxStreams,
				"total":       totalStreams,
				"lastRefresh": s.provider.GetLastRefresh().Format(time.RFC3339),
			},
		}

		c.JSON(200, metrics)
	}
}

func headContent(version string) template.HTML {
	if IsDebugMode() {
		return template.HTML(`<script src="/static/js/htmx.min.js?v=debug"></script>
		<script src="https://cdn.tailwindcss.com"></script>
    <script>
        tailwind.config = {
            darkMode: 'class',
            theme: {
                extend: {
                    colors: {
                        dark: {
                            bg: '#1a202c',
                            text: '#e2e8f0',
                        },
                    },
                },
            },
        }
    </script>
		<style type="text/tailwindcss">
			@layer utilities {
					.toggle-checkbox:checked {
							@apply: right-0 border-green-400;
							right: 0;
							border-color: #68D391;
					}
					.toggle-checkbox:checked + .toggle-label {
							@apply: bg-green-400;
							background-color: #68D391;
					}
			}
		</style>`)
	} else {
		return template.HTML(fmt.Sprintf(`<script src="/static/js/htmx.min.js?v=%[1]s"></script>
		<link href="/static/css/output.css?v=%[1]s" rel="stylesheet">`, version))
	}
}

func (s *Server) homePage() gin.HandlerFunc {
	return func(c *gin.Context) {
		streamInfoData := s.getStreamInfoData()
		streamInfoData["HeadContent"] = s.headContent
		if IsDebugMode() {
			streamInfoData["Version"] = "debug"
		} else {
			streamInfoData["Version"] = s.version
		}
		c.HTML(http.StatusOK, "base.html", streamInfoData)
	}
}

func (s *Server) getStreamInfoData() gin.H {
	return gin.H{
		"ActiveStreams": s.getActiveStreams(),
		"TotalStreams":  atomic.LoadInt64(&s.totalStreams),
		"Now":           time.Now(),
	}
}

func (s *Server) getStreamInfo() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.HTML(http.StatusOK, "stream_info.html", s.getStreamInfoData())
	}
}

func (s *Server) Start(provider *Provider) chan error {
	s.router.GET("/ping", func(c *gin.Context) {
		c.String(200, "PONG")
	})

	s.router.Use(s.streamTracker)

	s.router.GET("/", s.homePage())
	s.router.GET("/iptv.m3u", s.getIptvM3u())
	s.router.GET("/epg.xml", s.getEpgXML())
	s.router.GET(fmt.Sprintf("%s:channelId", channelURIPrefix), s.streamChannel())
	s.router.PUT("/refresh", s.refresh())
	s.router.GET("/debug", s.debug())
	s.router.GET("/stream-info", s.getStreamInfo())
	s.router.DELETE("/streams/:streamId", s.disconnectStream())
	s.router.StaticFS("/static", static.AssetFile())

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

var ErrTimeout = errors.New("timeout")

type TimeoutReader struct {
	r       io.Reader
	timeout time.Duration
}

func NewTimeoutReader(r io.Reader, timeout time.Duration) *TimeoutReader {
	return &TimeoutReader{r: r, timeout: timeout}
}

func (tr *TimeoutReader) Read(p []byte) (int, error) {
	ch := make(chan readResult)
	go func() {
		n, err := tr.r.Read(p)
		ch <- readResult{n: n, err: err}
	}()

	select {
	case res := <-ch:
		return res.n, res.err
	case <-time.After(tr.timeout):
		return 0, ErrTimeout
	}
}

type TimeoutWriter struct {
	w       io.Writer
	timeout time.Duration
}

func NewTimeoutWriter(w io.Writer, timeout time.Duration) *TimeoutWriter {
	return &TimeoutWriter{w: w, timeout: timeout}
}

func (tw *TimeoutWriter) Write(p []byte) (int, error) {
	ch := make(chan writeResult)
	go func() {
		n, err := tw.w.Write(p)
		ch <- writeResult{n: n, err: err}
	}()

	select {
	case res := <-ch:
		return res.n, res.err
	case <-time.After(tw.timeout):
		return 0, ErrTimeout
	}
}

type readResult struct {
	n   int
	err error
}

type writeResult struct {
	n   int
	err error
}
