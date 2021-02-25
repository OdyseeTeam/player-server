module github.com/lbryio/lbrytv-player

go 1.15

replace github.com/btcsuite/btcd => github.com/lbryio/lbrycrd.go v0.0.0-20200203050410-e1076f12bf19

replace github.com/floostack/transcoder => github.com/andybeletsky/transcoder v1.2.0

//replace github.com/lbryio/reflector.go => /home/niko/go/src/github.com/lbryio/reflector.go

require (
	github.com/aws/aws-sdk-go v1.23.19 // indirect
	github.com/aybabtme/iocontrol v0.0.0-20150809002002-ad15bcfc95a0
	github.com/benbjohnson/clock v1.0.3 // indirect
	github.com/c2h5oh/datasize v0.0.0-20200825124411-48ed595a09d2
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/getsentry/sentry-go v0.5.1
	github.com/gorilla/mux v1.7.4
	github.com/lbryio/ccache/v2 v2.0.7-0.20201103203756-4f264cc4f101
	github.com/lbryio/lbry.go/v2 v2.6.1-0.20200901183659-29574578c1c1
	github.com/lbryio/reflector.go v1.1.3-0.20210223142346-8cb73896199b
	github.com/lbryio/transcoder v0.4.7
	github.com/lbryio/types v0.0.0-20191228214437-05a22073b4ec
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/procfs v0.0.4 // indirect
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.6.1
	github.com/ybbus/jsonrpc v2.1.2+incompatible
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/appengine v1.6.5 // indirect
	google.golang.org/protobuf v1.24.0 // indirect
)
