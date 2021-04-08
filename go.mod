module github.com/lbryio/lbrytv-player

go 1.15

replace github.com/btcsuite/btcd => github.com/lbryio/lbrycrd.go v0.0.0-20200203050410-e1076f12bf19

replace github.com/floostack/transcoder => github.com/andybeletsky/transcoder v1.2.0

//replace github.com/lbryio/reflector.go => /home/niko/go/src/github.com/lbryio/reflector.go

require (
	github.com/aybabtme/iocontrol v0.0.0-20150809002002-ad15bcfc95a0
	github.com/benbjohnson/clock v1.0.3 // indirect
	github.com/c2h5oh/datasize v0.0.0-20200825124411-48ed595a09d2
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/getsentry/sentry-go v0.5.1
	github.com/golang/groupcache v0.0.0-20191027212112-611e8accdfc9 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/lbryio/ccache/v2 v2.0.7-0.20201103203756-4f264cc4f101
	github.com/lbryio/lbry.go/v2 v2.7.2-0.20210316000044-988178df5011
	github.com/lbryio/reflector.go v1.1.3-0.20210407024618-dc95351cf30c
	github.com/lbryio/transcoder v0.10.3
	github.com/lbryio/types v0.0.0-20201019032447-f0b4476ef386
	github.com/prometheus/client_golang v1.9.0
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/ybbus/jsonrpc v2.1.2+incompatible
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	google.golang.org/protobuf v1.24.0 // indirect
)

// replace github.com/lbryio/transcoder => /Users/silence/Documents/Lbry/Repos/transcoder
