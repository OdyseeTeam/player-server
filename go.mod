module github.com/lbryio/lbrytv-player

go 1.16

require (
	github.com/aybabtme/iocontrol v0.0.0-20150809002002-ad15bcfc95a0
	github.com/benbjohnson/clock v1.0.3 // indirect
	github.com/bluele/gcache v0.0.2
	github.com/c2h5oh/datasize v0.0.0-20200825124411-48ed595a09d2
	github.com/getsentry/sentry-go v0.5.1
	github.com/golang-jwt/jwt v3.2.1+incompatible
	github.com/gorilla/mux v1.8.0
	github.com/lbryio/lbry.go/v2 v2.7.2-0.20210416195322-6516df1418e3
	github.com/lbryio/reflector.go v1.1.3-0.20211214194950-ae0c7dd2bb76
	github.com/lbryio/transcoder v0.13.2
	github.com/lbryio/types v0.0.0-20201019032447-f0b4476ef386
	github.com/prometheus/client_golang v1.10.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.1.3
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.9.4 // indirect
	github.com/ybbus/jsonrpc v2.1.2+incompatible
	golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/tools v0.1.6 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
)

replace github.com/btcsuite/btcd => github.com/lbryio/lbrycrd.go v0.0.0-20200203050410-e1076f12bf19

replace github.com/floostack/transcoder => github.com/andybeletsky/transcoder v1.2.0

//replace github.com/lbryio/reflector.go => /home/niko/go/src/github.com/lbryio/reflector.go
//replace github.com/lbryio/transcoder => /home/niko/work/repositories/transcoder
//replace github.com/nikooo777/lbry-blobs-downloader => github.com/andybeletsky/lbry-blobs-downloader v1.0.4-fixed6
