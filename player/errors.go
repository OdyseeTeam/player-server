package player

import (
	"errors"
)

var (
	ErrPaidStream                      = errors.New("paid stream")
	ErrEdgeAuthenticationMisconfigured = errors.New("edge authentication misconfigured")
	ErrEdgeAuthenticationFailed        = errors.New("edge authentication failed")
	ErrEdgeCredentialsMissing          = errors.New("edge credentials missing")
	ErrClaimNotFound                   = errors.New("could not resolve stream URI")

	ErrSeekBeforeStart = errors.New("seeking before the beginning of file")
	ErrSeekOutOfBounds = errors.New("seeking out of bounds")
	ErrStreamSizeZero  = errors.New("stream size is zero")
)
