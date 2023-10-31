package player

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/OdyseeTeam/player-server/firewall"
	"github.com/OdyseeTeam/player-server/internal/iapi"
	"github.com/OdyseeTeam/player-server/internal/metrics"
	"github.com/OdyseeTeam/player-server/pkg/app"

	tclient "github.com/lbryio/transcoder/client"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// SpeechPrefix is root level prefix for speech URLs.
const SpeechPrefix = "/speech/"

const (
	paramDownload = "download"
	paramHashHLS  = "hash-hls" // Nested hash parameter for signed hls url to use with StackPath
	paramClientIP = "ip"       // Nested client IP parameter for hls urls to use with StackPath
	paramHash77   = "hash77"   // Nested hash parameter for signed url to use with CDN77
)

var (
	StreamWriteTimeout = uint(86400)
	playerName         = "unknown-player"
	reV5StartEndpoint  = regexp.MustCompile(`^/v5/streams/start/.+`)
	reV6StartEndpoint  = regexp.MustCompile(`^/v6/streams/.+(\.mp4)?$`)
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

var bannedIPs = map[string]bool{
	"96.76.237.222":   true,
	"45.47.236.87":    true,
	"154.53.32.121":   true,
	"51.222.12.22":    true,
	"65.108.133.222":  true,
	"135.181.178.92":  true,
	"65.21.95.58":     true,
	"80.129.211.95":   true,
	"5.161.101.49":    true,
	"199.217.105.250": true,
	"3.237.164.90":    true,
	"3.237.210.152":   true,
	"3.237.165.26":    true,
	"3.237.221.248":   true,
	"37.120.159.165":  true,
	"199.217.105.243": true,
	"79.137.105.150":  true,
	"89.187.177.54":   true,
	"5.161.108.85":    true,
	"5.252.23.106":    true,
	"207.244.91.166":  true,
	"198.98.52.25":    true,
	"207.244.91.131":  true,
	"175.182.108.229": true,
	"107.181.206.145": true,
}

var allowedReferrers = map[string]bool{
	"https://piped.kavin.rocks/": true,
	"https://piped.video/":       true,
	"https://www.gstatic.com/":   true,
	"http://localhost:9090/":     true,
}
var allowedTldReferrers = map[string]bool{
	"odysee.com": true,
	"odysee.tv":  true,
}
var allowedOrigins = map[string]bool{
	"https://odysee.com":         true,
	"https://neko.odysee.tv":     true,
	"https://salt.odysee.tv":     true,
	"https://kp.odysee.tv":       true,
	"https://inf.odysee.tv":      true,
	"https://stashu.odysee.tv":   true,
	"https://www.gstatic.com":    true,
	"https://odysee.ap.ngrok.io": true,
}
var allowedUserAgents = []string{
	"LBRY/",
	"Roku/",
}
var allowedSpecialHeaders = map[string]bool{"x-cf-lb-monitor": true}

var allowedXRequestedWith = "com.odysee.app"

// Handle is responsible for all HTTP media delivery via player module.
func (h *RequestHandler) Handle(c *gin.Context) {
	addCSPHeaders(c)
	addPoweredByHeaders(c)
	c.Header("player-request-method", c.Request.Method)
	if c.Request.Method == http.MethodHead {
		c.Header("Cache-Control", "no-store, No-cache")
	}

	var uri string
	var isSpeech bool
	if strings.HasPrefix(c.Request.URL.String(), SpeechPrefix) {
		// Speech stuff
		uri = c.Request.URL.String()[len(SpeechPrefix):]
		extStart := strings.LastIndex(uri, ".")
		if extStart >= 0 {
			uri = uri[:extStart]
		}
		if uri == "" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		isSpeech = true
		// Speech stuff over
	} else if c.Param("claim_name") != "" {
		uri = c.Param("claim_name") + "#" + c.Param("claim_id")
	} else {
		uri = c.Param("claim_id")
		if len(uri) != 40 {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
	}

	magicTimestamp, exists := c.GetQuery("magic")
	magicPass := false
	if exists && magicTimestamp != "" {
		unixT, err := strconv.Atoi(magicTimestamp)
		if err == nil {
			genesisTime := time.Unix(int64(unixT), 0)
			if time.Since(genesisTime) < 5*time.Minute {
				magicPass = true
			}
		}
	}

	flagged := true
	for header, v := range c.Request.Header {
		hasHeaderToCheck := header != "User-Agent" && header != "Referer" && header != "Origin" && header != "X-Requested-With"
		if hasHeaderToCheck && !allowedSpecialHeaders[strings.ToLower(header)] {
			continue
		}
		if strings.ToLower(header) == "origin" && allowedOrigins[v[0]] {
			flagged = false
			break
		}
		if strings.ToLower(header) == "referer" {
			if allowedReferrers[v[0]] {
				flagged = false
				break
			}
			//check if the referrer is from an allowed tld (weak check)
			for tld := range allowedTldReferrers {
				if strings.Contains(v[0], tld) {
					flagged = false
					break
				}
			}
		}

		if strings.ToLower(header) == "user-agent" {
			for _, ua := range allowedUserAgents {
				if strings.HasPrefix(v[0], ua) {
					flagged = false
					break
				}
			}
		}
		if allowedSpecialHeaders[strings.ToLower(header)] {
			flagged = false
			break
		}
		if strings.ToLower(header) == "x-requested-with" && v[0] == allowedXRequestedWith {
			flagged = false
			break
		}
	}
	//if the request is flagged and the magic pass is not set then we will not serve the request
	//with an exception for /v3/ endpoints for now
	flagged = !magicPass && flagged && !strings.HasPrefix(c.Request.URL.String(), "/api/v3/")

	//this is here temporarily due to abuse. a better solution will be found
	ip := c.ClientIP()
	if bannedIPs[ip] {
		c.AbortWithStatus(http.StatusTooManyRequests)
		return
	}
	if strings.Contains(c.Request.URL.String(), "Katmovie18") {
		c.String(http.StatusForbidden, "this content cannot be accessed")
		return
	}
	//end of abuse block

	blocked, err := iapi.GetBlockedContent()
	if err == nil {
		if blocked[c.Param("claim_id")] {
			c.String(http.StatusForbidden, "this content cannot be accessed")
			return
		}
	}
	isDownload, _ := strconv.ParseBool(c.Query(paramDownload))

	if isDownload {
		// log all headers for download requests
		//encode headers in a json string
		headers, err := json.MarshalIndent(c.Request.Header, "", "  ")
		if err == nil {
			logrus.Infof("download request for %s with IP %s and headers: %+v", uri, ip, string(headers))
		}
	}
	//don't allow downloads if either flagged or disabled
	if isDownload && (!h.player.options.downloadsEnabled || flagged) {
		c.String(http.StatusForbidden, "downloads are currently disabled")
		return
	}

	stream, err := h.player.ResolveStream(uri)
	addBreadcrumb(c.Request, "sdk", fmt.Sprintf("resolve %v", uri))
	if err != nil {
		metrics.ResolveFailures.Inc()
		processStreamError("resolve", uri, c.Writer, c.Request, err)
		return
	}
	hasValidChannel := stream.claim.SigningChannel != nil && stream.claim.SigningChannel.ClaimID != ""
	if hasValidChannel && blocked != nil && blocked[stream.claim.SigningChannel.ClaimID] {
		c.String(http.StatusForbidden, "this content cannot be accessed")
		return
	}
	abusiveIP, abuseCount := firewall.CheckAndRateLimitIp(ip, stream.ClaimID)
	if abusiveIP {
		Logger.Warnf("IP %s is abusing resources (count: %d): %s - %s", ip, abuseCount, stream.ClaimID, stream.claim.Name)
		if abuseCount > 10 {
			c.String(http.StatusTooManyRequests, "Try again later")
			return
		}
	}
	if isDownload && abuseCount > 2 {
		c.String(http.StatusTooManyRequests, "Try again later")
		return
	}

	err = h.player.VerifyAccess(stream, c)
	if err != nil {
		processStreamError("access", uri, c.Writer, c.Request, err)
		return
	}
	if flagged && !isSpeech {
		c.String(http.StatusUnauthorized, "this content cannot be accessed at the moment")
		return
	}

	if !isDownload && fitForTranscoder(c, stream) && h.player.tclient != nil {
		tcPath := h.player.tclient.GetPlaybackPath(c.Param("claim_id"), stream.hash)
		if tcPath != "" {
			metrics.StreamsDelivered.WithLabelValues(metrics.StreamTranscoded).Inc()
			c.Redirect(http.StatusPermanentRedirect, getPlaylistURL(c.Request.URL.Path, c.Request.URL.Query(), tcPath, stream))
			return
		}
	}

	if c.GetHeader("range") == "" {
		metrics.StreamsDelivered.WithLabelValues(metrics.StreamOriginal).Inc()
	}

	err = stream.PrepareForReading()
	addBreadcrumb(c.Request, "sdk", fmt.Sprintf("retrieve %v", uri))
	if err != nil {
		processStreamError("retrieval", uri, c.Writer, c.Request, err)
		return
	}

	writeHeaders(c, stream)

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
		err = h.player.Play(stream, c)
		if err != nil {
			processStreamError("playback", uri, c.Writer, c.Request, err)
			return
		}
	}
}

func (h *RequestHandler) HandleTranscodedFragment(c *gin.Context) {
	uri := c.Param("claim_id")
	addCSPHeaders(c)
	addPoweredByHeaders(c)
	metrics.StreamsRunning.WithLabelValues(metrics.StreamTranscoded).Inc()
	defer metrics.StreamsRunning.WithLabelValues(metrics.StreamTranscoded).Dec()
	blocked, err := iapi.GetBlockedContent()
	if err == nil {
		if blocked[uri] {
			c.String(http.StatusForbidden, "this content cannot be accessed")
			return
		}
	}
	size, err := h.player.tclient.PlayFragment(uri, c.Param("sd_hash"), c.Param("fragment"), c.Writer, c.Request)
	if err != nil {
		writeErrorResponse(c.Writer, http.StatusNotFound, err.Error())
	}
	metrics.TcOutBytes.Add(float64(size))
}

func writeHeaders(c *gin.Context, s *Stream) {
	c.Header("Content-Length", fmt.Sprintf("%v", s.Size))
	c.Header("Content-Type", s.ContentType)
	c.Header("Last-Modified", s.Timestamp().UTC().Format(http.TimeFormat))
	if c.Request.Method != http.MethodHead {
		c.Header("Cache-Control", "public, max-age=31536000")
	}

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

	if errors.Is(err, ErrPaidStream) {
		writeErrorResponse(w, http.StatusPaymentRequired, err.Error())
	} else if errors.Is(err, ErrClaimNotFound) {
		sendToSentry = false
		writeErrorResponse(w, http.StatusNotFound, err.Error())
	} else if errors.Is(err, ErrEdgeCredentialsMissing) {
		sendToSentry = false
		writeErrorResponse(w, http.StatusUnauthorized, err.Error())
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

func getPlaylistURL(fullPath string, query url.Values, tcPath string, stream *Stream) string {
	if strings.HasPrefix(fullPath, "/v5/streams/start/") {
		qs := ""
		if query.Get(paramHashHLS) != "" {
			qs = fmt.Sprintf("?ip=%s&hash=%s", query.Get(paramClientIP), query.Get(paramHashHLS))
		}
		return fmt.Sprintf("/v5/streams/hls/%s%s", tcPath, qs)
	} else if strings.HasPrefix(fullPath, "/v6/streams/") {
		path := fmt.Sprintf("/v6/streams/%s", tcPath)
		h := query.Get(paramHash77)
		if h != "" {
			path = "/" + h + path
		}
		return path
	}
	return fmt.Sprintf("/api/v4/streams/tc/%s/%s", stream.URL, tcPath)
}

func fitForTranscoder(c *gin.Context, s *Stream) bool {
	return (strings.HasPrefix(c.FullPath(), "/api/v4/") ||
		((reV5StartEndpoint.MatchString(c.FullPath()) || reV6StartEndpoint.MatchString(c.FullPath())) && c.Request.Method == http.MethodHead)) &&
		strings.HasPrefix(s.ContentType, "video/") && c.GetHeader("range") == ""
}
