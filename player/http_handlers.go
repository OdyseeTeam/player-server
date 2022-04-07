package player

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/OdyseeTeam/player-server/internal/metrics"
	"github.com/OdyseeTeam/player-server/pkg/app"

	tclient "github.com/lbryio/transcoder/client"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

const paramDownload = "download"

// SpeechPrefix is root level prefix for speech URLs.
const SpeechPrefix = "/speech/"

var (
	playerName         = "unknown-player"
	StreamWriteTimeout = uint(86400)
)

// RequestHandler is a HTTP request handler for player package.
type RequestHandler struct {
	player *Player
}

func init() {
	var err error

	playerName = os.Getenv("PLAYER_NAME")
	if playerName == "" {
		playerName, err = os.Hostname()
		if err != nil {
			playerName = "unknown-player"
		}
	}
}

// NewRequestHandler initializes an HTTP request handler with the provided Player instance.
func NewRequestHandler(p *Player) *RequestHandler {
	return &RequestHandler{p}
}

// Handle is responsible for all HTTP media delivery via player module.
func (h *RequestHandler) Handle(c *gin.Context) {
	addCSPHeaders(c)
	addPoweredByHeaders(c)
	var uri, token string

	// Speech stuff
	if strings.HasPrefix(c.Request.URL.String(), SpeechPrefix) {
		uri = c.Request.URL.String()[len(SpeechPrefix):]
		extStart := strings.LastIndex(uri, ".")
		if extStart >= 0 {
			uri = uri[:extStart]
		}
		if uri == "" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
	} else {
		uri = fmt.Sprintf("%s#%s", c.Param("claim_name"), c.Param("claim_id"))
		token = c.Param("token")
	}
	// Speech stuff over

	Logger.Infof("%s stream %v", c.Request.Method, uri)
	isDownload, _ := strconv.ParseBool(c.Query(paramDownload))
	if isDownload && !h.player.downloadsEnabled {
		c.String(http.StatusForbidden, "downloads are currently disabled")
		return
	}

	s, err := h.player.ResolveStream(uri)
	addBreadcrumb(c.Request, "sdk", fmt.Sprintf("resolve %v", uri))
	if err != nil {
		metrics.ResolveFailures.Inc()
		processStreamError("resolve", uri, c.Writer, c.Request, err)
		return
	}

	err = h.player.VerifyAccess(s, token)
	if err != nil {
		processStreamError("access", uri, c.Writer, c.Request, err)
		return
	}

	if !isDownload && fitForTranscoder(c, s) && h.player.tclient != nil {
		path := h.player.tclient.GetPlaybackPath(uri, s.hash)
		if path != "" {
			metrics.StreamsDelivered.WithLabelValues(metrics.StreamTranscoded).Inc()
			redirectToPlaylistURL(c, path)
			return
		}
	}

	if c.GetHeader("range") == "" {
		metrics.StreamsDelivered.WithLabelValues(metrics.StreamOriginal).Inc()
	}

	err = s.PrepareForReading()
	addBreadcrumb(c.Request, "sdk", fmt.Sprintf("retrieve %v", uri))
	if err != nil {
		processStreamError("retrieval", uri, c.Writer, c.Request, err)
		return
	}

	writeHeaders(c, s)

	conn, err := app.GetConnection(c.Request)
	if err != nil {
		Logger.Warn("can't set write timeout: ", err)
	} else {
		err = conn.SetWriteDeadline(time.Now().Add(time.Duration(StreamWriteTimeout) * time.Second))
		if err != nil {
			Logger.Error("can't set write timeout: ", err)
		}
	}

	switch c.Request.Method {
	case http.MethodHead:
		c.Status(http.StatusOK)
	case http.MethodGet:
		addBreadcrumb(c.Request, "player", fmt.Sprintf("play %v", uri))
		err = h.player.Play(s, c)
		if err != nil {
			processStreamError("playback", uri, c.Writer, c.Request, err)
			return
		}
	}
}

func (h *RequestHandler) HandleTranscodedFragment(c *gin.Context) {
	uri := fmt.Sprintf("%s#%s", c.Param("claim_name"), c.Param("claim_id"))
	addCSPHeaders(c)
	addPoweredByHeaders(c)
	metrics.StreamsRunning.WithLabelValues(metrics.StreamTranscoded).Inc()
	defer metrics.StreamsRunning.WithLabelValues(metrics.StreamTranscoded).Dec()
	err := h.player.tclient.PlayFragment(uri, c.Param("sd_hash"), c.Param("fragment"), c.Writer, c.Request) //todo change transcoder to accept Gin Context
	if err != nil {
		writeErrorResponse(c.Writer, http.StatusNotFound, err.Error())
	}
}

func writeHeaders(c *gin.Context, s *Stream) {
	c.Header("Content-Length", fmt.Sprintf("%v", s.Size))
	c.Header("Content-Type", s.ContentType)
	c.Header("Cache-Control", "public, max-age=31536000")
	c.Header("Last-Modified", s.Timestamp().UTC().Format(http.TimeFormat))

	isDownload, _ := strconv.ParseBool(c.Query(paramDownload))
	if isDownload {

		filename := regexp.MustCompile(`[^\p{L}\d\-\._ ]+`).ReplaceAllString(s.Filename(), "")
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename)))
	}
}

func processStreamError(errorType string, uri string, w http.ResponseWriter, r *http.Request, err error) {
	sendToSentry := true

	if err == tclient.ErrChannelNotEnabled {
		return
	}

	if w == nil {
		Logger.Errorf("%s stream GET - %s error: %v", uri, errorType, err)
		return
	}

	Logger.Errorf("%s stream %v - %s error: %v", r.Method, uri, errorType, err)

	if errors.Is(err, errPaidStream) {
		writeErrorResponse(w, http.StatusPaymentRequired, err.Error())
	} else if errors.Is(err, errStreamNotFound) {
		sendToSentry = false
		writeErrorResponse(w, http.StatusNotFound, err.Error())
	} else if strings.Contains(err.Error(), "blob not found") {
		sendToSentry = false
		writeErrorResponse(w, http.StatusServiceUnavailable, err.Error())
	} else if strings.Contains(err.Error(), "hash in response does not match") {
		writeErrorResponse(w, http.StatusServiceUnavailable, err.Error())
	} else if strings.Contains(err.Error(), "token contains an invalid number of segments") {
		writeErrorResponse(w, http.StatusUnauthorized, err.Error())
	} else if strings.Contains(err.Error(), "crypto/rsa: verification error") {
		writeErrorResponse(w, http.StatusUnauthorized, err.Error())
	} else if strings.Contains(err.Error(), "token is expired") {
		writeErrorResponse(w, http.StatusGone, err.Error())
	} else {
		// logger.CaptureException(err, map[string]string{"uri": uri})
		writeErrorResponse(w, http.StatusInternalServerError, err.Error())
	}

	if hub := sentry.GetHubFromContext(r.Context()); hub != nil && sendToSentry && err != nil {
		hub.CaptureException(err)
	}
}

func writeErrorResponse(w http.ResponseWriter, statusCode int, msg string) {
	w.WriteHeader(statusCode)
	w.Write([]byte(msg))
}

func addBreadcrumb(r *http.Request, category, message string) {
	if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
		hub.Scope().AddBreadcrumb(&sentry.Breadcrumb{
			Category: category,
			Message:  message,
		}, 99)
	}
}

func addPoweredByHeaders(c *gin.Context) {
	c.Header("X-Powered-By", playerName)
	c.Header("Access-Control-Expose-Headers", "X-Powered-By")
}

func addCSPHeaders(c *gin.Context) {
	c.Header("Report-To", `{"group":"default","max_age":31536000,"endpoints":[{"url":"https://6fd448c230d0731192f779791c8e45c3.report-uri.com/a/d/g"}],"include_subdomains":true}`)
	c.Header("Content-Security-Policy", "script-src 'none'; report-uri https://6fd448c230d0731192f779791c8e45c3.report-uri.com/r/d/csp/enforce; report-to default")
}

func redirectToPlaylistURL(c *gin.Context, path string) {
	c.Redirect(http.StatusPermanentRedirect, fmt.Sprintf("/api/v4/streams/tc/%v", path))
}

func fitForTranscoder(c *gin.Context, s *Stream) bool {
	return strings.HasPrefix(c.FullPath(), "/api/v4/") &&
		strings.HasPrefix(s.ContentType, "video/") && c.GetHeader("range") == ""
}
