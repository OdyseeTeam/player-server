package player

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/mux"
)

const ParamDownload = "download"

// RequestHandler is a HTTP request handler for player package.
type RequestHandler struct {
	player *Player
}

// NewRequestHandler initializes a HTTP request handler with the provided Player instance.
func NewRequestHandler(p *Player) *RequestHandler {
	return &RequestHandler{p}
}

func (h RequestHandler) getURI(r *http.Request) string {
	vars := mux.Vars(r)
	return fmt.Sprintf("%s#%s", vars["claim_name"], vars["claim_id"])
}

func (h RequestHandler) getToken(r *http.Request) string {
	return mux.Vars(r)["token"]
}

func (h RequestHandler) writeErrorResponse(w http.ResponseWriter, statusCode int, msg string) {
	w.WriteHeader(statusCode)
	w.Write([]byte(msg))
}

func (h RequestHandler) writeHeaders(w http.ResponseWriter, r *http.Request, s *Stream) {
	playerName := os.Getenv("PLAYER_NAME")
	var err error
	if playerName == "" {
		playerName, err = os.Hostname()
		if err != nil {
			playerName = "unknown-player"
		}
	}

	header := w.Header()
	header.Set("Content-Length", fmt.Sprintf("%v", s.Size))
	header.Set("Content-Type", s.ContentType)
	header.Set("Cache-Control", "public, max-age=31536000")
	header.Set("Last-Modified", s.Timestamp().UTC().Format(http.TimeFormat))
	header.Set("X-Powered-By", playerName)
	header.Set("Access-Control-Expose-Headers", "X-Powered-By")
	if r.URL.Query().Get(ParamDownload) != "" {
		header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%v", s.Filename()))
	}
}

func (h RequestHandler) processStreamError(w http.ResponseWriter, uri string, err error) {
	if errors.Is(err, errPaidStream) {
		h.writeErrorResponse(w, http.StatusPaymentRequired, err.Error())
	} else if errors.Is(err, errStreamNotFound) {
		h.writeErrorResponse(w, http.StatusNotFound, err.Error())
	} else if strings.Contains(err.Error(), "blob not found") {
		h.writeErrorResponse(w, http.StatusServiceUnavailable, err.Error())
	} else if strings.Contains(err.Error(), "hash in response does not match") {
		h.writeErrorResponse(w, http.StatusServiceUnavailable, err.Error())
	} else if strings.Contains(err.Error(), "token contains an invalid number of segments") {
		h.writeErrorResponse(w, http.StatusUnauthorized, err.Error())
	} else if strings.Contains(err.Error(), "crypto/rsa: verification error") {
		h.writeErrorResponse(w, http.StatusUnauthorized, err.Error())
	} else if strings.Contains(err.Error(), "token is expired") {
		h.writeErrorResponse(w, http.StatusGone, err.Error())
	} else {
		// logger.CaptureException(err, map[string]string{"uri": uri})
		h.writeErrorResponse(w, http.StatusInternalServerError, err.Error())
	}
}

func addBreadcrumb(r *http.Request, category, message string) {
	if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
		hub.Scope().AddBreadcrumb(&sentry.Breadcrumb{
			Category: category,
			Message:  message,
		}, 99)
	}
}

func logError(r *http.Request, err error) {
	if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
		hub.CaptureException(err)
	}
}

// Handle is responsible for all HTTP media delivery via player module.
func (h *RequestHandler) Handle(w http.ResponseWriter, r *http.Request) {
	uri := h.getURI(r)
	token := h.getToken(r)
	Logger.Infof("GET stream %v", uri) // , users.GetIPAddressForRequest(r))

	s, err := h.player.ResolveStream(uri)
	addBreadcrumb(r, "sdk", fmt.Sprintf("resolve %v", uri))
	if err != nil {
		Logger.Errorf("GET stream %v - resolve error: %v", uri, err)
		logError(r, err)
		h.processStreamError(w, uri, err)
		return
	}

	err = h.player.VerifyAccess(s, token)
	if err != nil {
		Logger.Errorf("GET stream %v - access error: %v", uri, err)
		logError(r, err)
		h.processStreamError(w, uri, err)
		return
	}

	err = s.PrepareForReading()
	addBreadcrumb(r, "sdk", fmt.Sprintf("retrieve %v", uri))
	if err != nil {
		Logger.Errorf("GET stream %v - retrieval error: %v", uri, err)
		logError(r, err)
		h.processStreamError(w, uri, err)
		return
	}

	h.writeHeaders(w, r, s)

	addBreadcrumb(r, "player", fmt.Sprintf("play %v", uri))
	err = h.player.Play(s, w, r)
	if err != nil {
		Logger.Errorf("GET stream %v - playback error: %v", uri, err)
		logError(r, err)
		h.processStreamError(w, uri, err)
		return
	}
}

// HandleHead handlers OPTIONS requests for media.
func (h *RequestHandler) HandleHead(w http.ResponseWriter, r *http.Request) {
	uri := h.getURI(r)
	token := h.getToken(r)

	s, err := h.player.ResolveStream(uri)
	if err != nil {
		h.processStreamError(w, uri, err)
		return
	}

	err = h.player.VerifyAccess(s, token)
	if err != nil {
		h.processStreamError(w, uri, err)
		return
	}

	err = s.PrepareForReading()
	if err != nil {
		h.processStreamError(w, uri, err)
		return
	}

	h.writeHeaders(w, r, s)
	w.WriteHeader(http.StatusOK)
}
