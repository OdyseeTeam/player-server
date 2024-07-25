package player

import (
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/OdyseeTeam/player-server/internal/metrics"
	"github.com/OdyseeTeam/player-server/pkg/logger"
	"github.com/OdyseeTeam/player-server/pkg/paid"
	"github.com/prometheus/client_golang/prometheus"

	tclient "github.com/OdyseeTeam/transcoder/client"
	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"

	"github.com/bluele/gcache"
	"github.com/gin-gonic/gin"
)

const (
	edgeTokenHeader      = "Authorization"
	edgeTokenPrefix      = "Token "
	resolveCacheDuration = 5 * time.Minute
	defaultSdkAddress    = "https://api.na-backend.odysee.com/api/v1/proxy"
)

var (
	Logger  = logger.GetLogger()
	reClaim = regexp.MustCompile("^[a-z0-9]{40}$")
)

type PlayerOptions struct {
	edgeToken        string
	lbrynetAddress   string
	downloadsEnabled bool
	prefetch         bool
}

// Player is an entry-point object to the new player package.
type Player struct {
	lbrynetClient *ljsonrpc.Client
	blobSource    *HotCache
	resolveCache  gcache.Cache
	tclient       *tclient.Client
	TCVideoPath   string

	options PlayerOptions
}

func WithEdgeToken(token string) func(options *PlayerOptions) {
	return func(options *PlayerOptions) {
		options.edgeToken = token
	}
}

func WithLbrynetServer(address string) func(options *PlayerOptions) {
	return func(options *PlayerOptions) {
		options.lbrynetAddress = address
	}
}

func WithDownloads(allow bool) func(options *PlayerOptions) {
	return func(options *PlayerOptions) {
		options.downloadsEnabled = allow
	}
}

func WithPrefetch(enabled bool) func(options *PlayerOptions) {
	return func(options *PlayerOptions) {
		options.prefetch = enabled
	}
}

// NewPlayer initializes an instance with optional BlobStore.
func NewPlayer(hotCache *HotCache, optionFuncs ...func(*PlayerOptions)) *Player {
	options := &PlayerOptions{
		lbrynetAddress:   defaultSdkAddress,
		downloadsEnabled: true,
	}

	for _, optionFunc := range optionFuncs {
		optionFunc(options)
	}

	lbrynetClient := ljsonrpc.NewClient(options.lbrynetAddress)
	lbrynetClient.SetRPCTimeout(10 * time.Second)
	return &Player{
		lbrynetClient: lbrynetClient,
		blobSource:    hotCache,
		resolveCache:  gcache.New(10000).ARC().Build(),
		options:       *options,
	}
}

func (p *Player) AddTranscoderClient(c *tclient.Client, path string) {
	p.tclient = c
	p.TCVideoPath = path
}

// Play delivers requested URI onto the supplied http.ResponseWriter.
func (p *Player) Play(s *Stream, c *gin.Context) error {
	metrics.StreamsRunning.WithLabelValues(metrics.StreamOriginal).Inc()
	defer metrics.StreamsRunning.WithLabelValues(metrics.StreamOriginal).Dec()
	ServeStream(c, s)
	return nil
}

// ResolveStream resolves provided URI by calling the SDK.
func (p *Player) ResolveStream(claimId string) (*Stream, error) {
	start := time.Now()
	defer func(t time.Time) {
		metrics.ResolveTimeMS.Observe(float64(time.Since(t).Milliseconds()))
	}(start)

	var claim *ljsonrpc.Claim

	cachedClaim, cErr := p.resolveCache.Get(claimId)
	if cErr != nil {
		var err error
		claim, err = p.resolve(claimId)
		if err != nil {
			return nil, err
		}

		maxDepth := 5
		for i := 0; i < maxDepth; i++ {
			repost := claim.Value.GetRepost()
			if repost == nil {
				break
			}
			if repost.ClaimHash == nil {
				return nil, errors.New("repost has no claim hash")
			}

			claimID := hex.EncodeToString(rev(repost.ClaimHash))
			_, err := p.resolve(claimID)
			if err != nil {
				if errors.Is(err, ErrClaimNotFound) {
					return nil, errors.New("reposted claim not found")
				}
				return nil, err
			}
		}
		metrics.ResolveSuccesses.WithLabelValues(metrics.ResolveSourceOApi).Inc()
		_ = p.resolveCache.SetWithExpire(claimId, claim, resolveCacheDuration)
	} else {
		metrics.ResolveSuccessesDuration.WithLabelValues(metrics.ResolveSourceCache).Observe(float64(time.Since(start)))
		metrics.ResolveSuccesses.WithLabelValues(metrics.ResolveSourceCache).Inc()
		claim = cachedClaim.(*ljsonrpc.Claim)
	}

	if claim.Value.GetStream() == nil {
		return nil, errors.New("claim is not stream")
	}
	if claim.Value.GetStream().GetSource() == nil {
		return nil, errors.New("stream has no source")
	}

	return NewStream(p, claim), nil
}

// resolve the claim
func (p *Player) resolve(claimID string) (*ljsonrpc.Claim, error) {
	generalFailureLabels := prometheus.Labels{
		metrics.ResolveSource: metrics.ResolveSourceOApi,
		metrics.ResolveKind:   metrics.ResolveFailureGeneral,
	}
	notFoundFailureLabels := prometheus.Labels{
		metrics.ResolveSource: metrics.ResolveSourceOApi,
		metrics.ResolveKind:   metrics.ResolveFailureClaimNotFound,
	}

	start := time.Now()

	// TODO: Get rid of the resolve call when ClaimSearchArgs acquires URI param
	if !reClaim.MatchString(claimID) {
		resolved, err := p.lbrynetClient.Resolve(claimID)
		if err != nil {
			metrics.ResolveFailuresDuration.With(generalFailureLabels).Observe(float64(time.Since(start)))
			metrics.ResolveFailures.With(generalFailureLabels).Inc()
			return nil, err
		}

		claim := (*resolved)[claimID]
		if claim.CanonicalURL == "" {
			metrics.ResolveFailuresDuration.With(notFoundFailureLabels).Observe(float64(time.Since(start)))
			metrics.ResolveFailures.With(notFoundFailureLabels).Inc()
			return nil, ErrClaimNotFound
		}
		return &claim, nil
	}
	resp, err := p.lbrynetClient.ClaimSearch(ljsonrpc.ClaimSearchArgs{ClaimID: &claimID, PageSize: 1, Page: 1})
	if err != nil {
		metrics.ResolveFailuresDuration.With(prometheus.Labels{
			metrics.ResolveSource: metrics.ResolveSourceOApi,
			metrics.ResolveKind:   metrics.ResolveFailureGeneral,
		}).Observe(float64(time.Since(start)))
		metrics.ResolveFailures.With(prometheus.Labels{
			metrics.ResolveSource: metrics.ResolveSourceOApi,
			metrics.ResolveKind:   metrics.ResolveFailureGeneral,
		}).Inc()
		return nil, err
	}
	if len(resp.Claims) == 0 {
		metrics.ResolveFailuresDuration.With(prometheus.Labels{
			metrics.ResolveSource: metrics.ResolveSourceOApi,
			metrics.ResolveKind:   metrics.ResolveFailureClaimNotFound,
		}).Observe(float64(time.Since(start)))
		metrics.ResolveFailures.With(prometheus.Labels{
			metrics.ResolveSource: metrics.ResolveSourceOApi,
			metrics.ResolveKind:   metrics.ResolveFailureClaimNotFound,
		}).Inc()
		return nil, ErrClaimNotFound
	}
	return &resp.Claims[0], nil
}

// VerifyAccess checks if the stream is protected and the token supplied matched the stream
func (p *Player) VerifyAccess(stream *Stream, ctx *gin.Context) error {
	protectedMap := map[string]bool{
		"c:members-only": true,
		"c:rental":       true,
		"c:purchase":     true,
		"c:unlisted":     true,
	}
	protectedWithTimeMap := map[string]bool{
		"c:scheduled:show": true,
		"c:scheduled:hide": true,
	}
	for _, t := range stream.Claim.Value.Tags {
		if protectedMap[t] ||
			strings.HasPrefix(t, "purchase:") ||
			strings.HasPrefix(t, "rental:") ||
			(protectedWithTimeMap[t] && stream.Claim.Value.GetStream().ReleaseTime > time.Now().Unix()) {
			th := ctx.Request.Header.Get(edgeTokenHeader)
			if th == "" {
				return ErrEdgeCredentialsMissing
			}
			if p.options.edgeToken == "" {
				return ErrEdgeAuthenticationMisconfigured
			}
			if strings.TrimPrefix(th, edgeTokenPrefix) != p.options.edgeToken {
				return ErrEdgeAuthenticationFailed
			}
			return nil
		}
	}

	token := ctx.Param("token")
	if stream.resolvedStream.Fee == nil || stream.resolvedStream.Fee.Amount <= 0 {
		return nil
	}

	Logger.WithField("uri", stream.URI).Info("paid stream requested")
	if token == "" {
		return ErrPaidStream
	}
	if err := paid.VerifyStreamAccess(strings.Replace(stream.URI(), "#", "/", 1), token); err != nil {
		return err
	}
	return nil
}

func rev(b []byte) []byte {
	r := make([]byte, len(b))
	for left, right := 0, len(b)-1; left < right; left, right = left+1, right-1 {
		r[left], r[right] = b[right], b[left]
	}
	return r
}
