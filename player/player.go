package player

import (
	"encoding/hex"
	"errors"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/OdyseeTeam/player-server/internal/metrics"
	"github.com/OdyseeTeam/player-server/pkg/logger"
	"github.com/OdyseeTeam/player-server/pkg/paid"

	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"
	tclient "github.com/lbryio/transcoder/client"

	"github.com/bluele/gcache"
	"github.com/gin-gonic/gin"
)

const (
	edgeTokenHeader = "Authorization"
	edgeTokenPrefix = "Token "
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
		lbrynetAddress:   "http://localhost:5279",
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
func (p *Player) ResolveStream(uri string) (*Stream, error) {
	defer func(t time.Time) {
		metrics.ResolveTimeMS.Observe(float64(time.Since(t).Milliseconds()))
	}(time.Now())

	var claim *ljsonrpc.Claim

	cachedClaim, err := p.resolveCache.Get(uri)
	if err == nil {
		claim = cachedClaim.(*ljsonrpc.Claim)
	} else {
		var err error
		claim, err = p.resolve(uri)
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
		metrics.ResolveSuccesses.Inc()
		_ = p.resolveCache.SetWithExpire(uri, claim, time.Duration(rand.Intn(5)+5)*time.Minute) // random time between 5 and 10 min, to spread load on wallet servers
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
	// TODO: Get rid of the resolve call when ClaimSearchArgs acquires URI param
	if !reClaim.MatchString(claimID) {
		resolved, err := p.lbrynetClient.Resolve(claimID)
		if err != nil {
			return nil, err
		}

		claim := (*resolved)[claimID]
		if claim.CanonicalURL == "" {
			return nil, ErrClaimNotFound
		}
		return &claim, nil
	}
	resp, err := p.lbrynetClient.ClaimSearch(ljsonrpc.ClaimSearchArgs{ClaimID: &claimID, PageSize: 1, Page: 1})
	if err != nil {
		return nil, err
	}
	if len(resp.Claims) == 0 {
		return nil, ErrClaimNotFound
	}
	return &resp.Claims[0], nil
}

// VerifyAccess checks if the stream is paid and the token supplied matched the stream
func (p *Player) VerifyAccess(stream *Stream, ctx *gin.Context) error {
	for _, t := range stream.claim.Value.Tags {
		if t == "c:members-only" || t == "c:rental" || t == "c:purchase" || strings.HasPrefix(t, "purchase:") || strings.HasPrefix(t, "rental:") {
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
