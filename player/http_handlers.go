package player

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/lbryio/lbrytv-player/internal/metrics"
	"github.com/lbryio/transcoder/video"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/mux"
)

const paramDownload = "download"

// SpeechPrefix is root level prefix for speech URLs.
const SpeechPrefix = "/speech/"

var playerName = "unknown-player"

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

// NewRequestHandler initializes a HTTP request handler with the provided Player instance.
func NewRequestHandler(p *Player) *RequestHandler {
	return &RequestHandler{p}
}

// Handle is responsible for all HTTP media delivery via player module.
func (h *RequestHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var uri, token string

	// Speech stuff
	if strings.HasPrefix(r.URL.String(), SpeechPrefix) {
		uri = r.URL.String()[len(SpeechPrefix):]
		extStart := strings.LastIndex(uri, ".")
		if extStart >= 0 {
			uri = uri[:extStart]
		}
		if uri == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	} else {
		vars := mux.Vars(r)
		uri = fmt.Sprintf("%s#%s", vars["claim_name"], vars["claim_id"])
		token = vars["token"]
	}
	// Speech stuff over

	Logger.Infof("%s stream %v", r.Method, uri)

	s, err := h.player.ResolveStream(uri)
	addBreadcrumb(r, "sdk", fmt.Sprintf("resolve %v", uri))
	if err != nil {
		processStreamError("resolve", uri, w, r, err)
		return
	}

	err = h.player.VerifyAccess(s, token)
	if err != nil {
		processStreamError("access", uri, w, r, err)
		return
	}

	if r.URL.Query().Get(paramDownload) != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%v", s.Filename()))
	} else if fitForTranscoder(r, s) && h.player.tclient != nil {
		path := h.player.tclient.GetPlaybackPath(uri, s.hash)
		fmt.Println("got path", path)
		if path != "" {
			metrics.StreamsDelivered.WithLabelValues(metrics.StreamTranscoded).Inc()
			redirectToPlaylistURL(w, r, path)
			return
		}
	}

	metrics.StreamsDelivered.WithLabelValues(metrics.StreamOriginal).Inc()

	err = s.PrepareForReading()
	addBreadcrumb(r, "sdk", fmt.Sprintf("retrieve %v", uri))
	if err != nil {
		processStreamError("retrieval", uri, w, r, err)
		return
	}

	writeHeaders(w, r, s)

	switch r.Method {
	case http.MethodHead:
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		addBreadcrumb(r, "player", fmt.Sprintf("play %v", uri))
		err = h.player.Play(s, w, r)
		if err != nil {
			processStreamError("playback", uri, w, r, err)
			return
		}
	}
}

func (h *RequestHandler) HandleTranscodedFragment(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	uri := fmt.Sprintf("%s#%s", v["claim_name"], v["claim_id"])

	addPoweredByHeaders(w)
	metrics.StreamsRunning.WithLabelValues(metrics.StreamTranscoded).Inc()
	defer metrics.StreamsRunning.WithLabelValues(metrics.StreamTranscoded).Dec()
	h.player.tclient.PlayFragment(uri, v["sd_hash"], v["fragment"], w, r)
}

func writeHeaders(w http.ResponseWriter, r *http.Request, s *Stream) {
	header := w.Header()
	header.Set("Content-Length", fmt.Sprintf("%v", s.Size))
	header.Set("Content-Type", s.ContentType)
	header.Set("Cache-Control", "public, max-age=31536000")
	header.Set("Last-Modified", s.Timestamp().UTC().Format(http.TimeFormat))
	addPoweredByHeaders(w)
	if r.URL.Query().Get(paramDownload) != "" {
		filename := regexp.MustCompile(`[^\p{L}\d\-\._ ]+`).ReplaceAllString(s.Filename(), "")
		header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename)))
	}
}

func processStreamError(errorType string, uri string, w http.ResponseWriter, r *http.Request, err error) {
	sendToSentry := true

	if err == video.ErrChannelNotEnabled {
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

func addPoweredByHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Powered-By", playerName)
	w.Header().Set("Access-Control-Expose-Headers", "X-Powered-By")
}

func redirectToPlaylistURL(w http.ResponseWriter, r *http.Request, path string) {
	prefix := "http://"
	host := playerName

	if match, _ := regexp.MatchString(`^player\d+$`, host); match {
		host += ".lbryplayer.xyz"
		prefix = "https://"
	}

	url := fmt.Sprintf("%v/api/v4/streams/tc/%v", host, path)
	http.Redirect(w, r, prefix+url, http.StatusPermanentRedirect)
}

func fitForTranscoder(r *http.Request, s *Stream) bool {
	return strings.HasPrefix(r.URL.Path, "/api/v4/") &&
		strings.HasPrefix(s.ContentType, "video/") &&
		r.Header.Get("range") == ""
}
