package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/OdyseeTeam/player-server/internal/config"
	"github.com/OdyseeTeam/player-server/internal/metrics"
	"github.com/OdyseeTeam/player-server/internal/version"
	"github.com/OdyseeTeam/player-server/pkg/app"
	"github.com/OdyseeTeam/player-server/pkg/logger"
	"github.com/OdyseeTeam/player-server/pkg/paid"
	"github.com/OdyseeTeam/player-server/player"
	"github.com/lbryio/reflector.go/server/http3"

	tclient "github.com/OdyseeTeam/transcoder/client"
	"github.com/lbryio/lbry.go/v2/stream"
	"github.com/lbryio/reflector.go/server/peer"
	"github.com/lbryio/reflector.go/store"

	"github.com/c2h5oh/datasize"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var Logger = logger.GetLogger()

var (
	bindAddress    string
	enablePrefetch bool
	enableProfile  bool
	verboseOutput  bool
	allowDownloads bool
	lbrynetAddress string
	paidPubKey     string

	upstreamReflector  string
	upstreamProtocol   string
	cloudFrontEndpoint string
	diskCacheDir       string
	diskCacheSize      string
	hotCacheSize       string

	transcoderVideoPath    string
	transcoderVideoSize    string
	transcoderAddr         string
	transcoderRemoteServer string

	edgeToken string

	rootCmd = &cobra.Command{
		Use:     "odysee_player",
		Short:   "media server for odysee.com",
		Version: version.FullName(),
		Run:     run,
	}
)

func init() {
	rootCmd.Flags().StringVar(&bindAddress, "bind", "0.0.0.0:8080", "address to bind HTTP server to")
	rootCmd.Flags().StringVar(&lbrynetAddress, "lbrynet", "https://api.na-backend.odysee.com/api/v1/proxy", "lbrynet server URL")
	rootCmd.Flags().StringVar(&paidPubKey, "paid_pubkey", "https://api.na-backend.odysee.com/api/v1/paid/pubkey", "pubkey for playing paid content")

	rootCmd.Flags().UintVar(&player.StreamWriteTimeout, "http-stream-write-timeout", player.StreamWriteTimeout, "write timeout for stream http requests (seconds)")
	rootCmd.Flags().UintVar(&app.WriteTimeout, "http-write-timeout", app.WriteTimeout, "write timeout for http requests (seconds)")
	rootCmd.Flags().UintVar(&app.ReadTimeout, "http-read-timeout", app.ReadTimeout, "read timeout for http requests (seconds)")

	rootCmd.Flags().BoolVar(&enablePrefetch, "prefetch", false, "enable prefetch for blobs")
	rootCmd.Flags().BoolVar(&enableProfile, "profile", false, fmt.Sprintf("enable profiling server at %v", player.ProfileRoutePath))
	rootCmd.Flags().BoolVar(&verboseOutput, "verbose", false, "enable verbose logging")
	rootCmd.Flags().BoolVar(&allowDownloads, "allow-downloads", true, "enable stream downloads")

	rootCmd.Flags().StringVar(&upstreamReflector, "upstream-reflector", "", "host:port of a reflector server where blobs are fetched from")
	rootCmd.Flags().StringVar(&upstreamProtocol, "upstream-protocol", "http", "protocol used to fetch blobs from another upstream reflector server (tcp/http3/http)")

	rootCmd.Flags().StringVar(&cloudFrontEndpoint, "cloudfront-endpoint", "", "CloudFront edge endpoint for standard HTTP retrieval")
	rootCmd.Flags().StringVar(&diskCacheDir, "disk-cache-dir", "", "enable disk cache, storing blobs in dir")
	rootCmd.Flags().StringVar(&diskCacheSize, "disk-cache-size", "100MB", "max size of disk cache: 16GB, 500MB, etc.")
	rootCmd.Flags().StringVar(&hotCacheSize, "hot-cache-size", "50MB", "max size for in-memory cache: 16GB, 500MB, etc")
	rootCmd.Flags().StringVar(&transcoderVideoPath, "transcoder-video-path", "", "path to store transcoded videos")
	rootCmd.Flags().StringVar(&transcoderVideoSize, "transcoder-video-size", "200GB", "max size of transcoder video storage")
	rootCmd.Flags().StringVar(&transcoderAddr, "transcoder-addr", "", "transcoder API address")
	rootCmd.Flags().StringVar(&transcoderRemoteServer, "transcoder-remote-server", "", "remote transcoder storage server URL")

	rootCmd.Flags().UintVar(&player.PrefetchCount, "prefetch-count", player.DefaultPrefetchLen, "how many blobs to retrieve from origin in advance")

	rootCmd.Flags().StringVar(&config.UserName, "config-username", "lbry", "Username to access the config endpoint with")
	rootCmd.Flags().StringVar(&config.Password, "config-password", "lbry", "Password to access the config endpoint with")
	rootCmd.Flags().Float64Var(&player.ThrottleScale, "throttle-scale", 1.5, "Throttle scale to rate limit in MB/s, only the 1.2 in 1.2MB/s")
	rootCmd.Flags().BoolVar(&player.ThrottleSwitch, "throttle-enabled", true, "Enables throttling")

	rootCmd.Flags().StringVar(&edgeToken, "edge-token", "", "Edge token for delivering purchased/rented streams")
}

func run(cmd *cobra.Command, args []string) {
	initLogger()
	defer logger.Flush()

	initPubkey()

	blobSource := getBlobSource()

	p := player.NewPlayer(
		initHotCache(blobSource),
		player.WithLbrynetServer(lbrynetAddress),
		player.WithDownloads(allowDownloads),
		player.WithPrefetch(enablePrefetch),
		player.WithEdgeToken(edgeToken),
	)

	var tcsize datasize.ByteSize
	err := tcsize.UnmarshalText([]byte(transcoderVideoSize))
	if err != nil {
		Logger.Fatal(err)
	}
	if transcoderVideoPath != "" && tcsize > 0 && transcoderAddr != "" {
		err := os.Mkdir(transcoderVideoPath, os.ModePerm)
		if err != nil && !os.IsExist(err) {
			Logger.Fatal(err)
		}

		tCfg := tclient.Configure().
			VideoPath(transcoderVideoPath).
			Server(transcoderAddr).
			CacheSize(int64(tcsize)).
			ItemsToPrune(10)
		if verboseOutput {
			tCfg = tCfg.LogLevel(tclient.Dev)
		}
		if transcoderRemoteServer != "" {
			tCfg = tCfg.RemoteServer(transcoderRemoteServer)
		}
		c := tclient.New(tCfg)
		//TODO: this can probably be in a separate go routine so that startup isn't blocked
		n, err := c.RestoreCache()
		if err != nil {
			Logger.Error(err)
		} else {
			Logger.Infof("restored %v items into transcoder cache", n)
		}

		p.AddTranscoderClient(&c, transcoderVideoPath)
	}

	a := app.New(app.Opts{Address: bindAddress, BlobStore: blobSource, EdgeToken: edgeToken})

	metrics.InstallRoute(a.Router)
	player.InstallPlayerRoutes(a.Router, p)
	config.InstallConfigRoute(a.Router)
	if enableProfile {
		player.InstallProfilingRoutes(a.Router)
	}

	a.Start()
	a.ServeUntilShutdown()
}

func initHotCache(origin store.BlobStore) *player.HotCache {
	var hotCacheBytes datasize.ByteSize
	err := hotCacheBytes.UnmarshalText([]byte(hotCacheSize))
	if err != nil {
		Logger.Fatal(err)
	}
	if hotCacheBytes <= 0 {
		Logger.Fatal("hot cache size must be greater than 0. if you want to disable hot cache, you'll have to do a bit of coding")
	}

	metrics.PlayerCacheInfo(hotCacheBytes.Bytes())
	unencryptedCache := player.NewDecryptedCache(origin)
	return player.NewHotCache(*unencryptedCache, int64(hotCacheBytes.Bytes()))
}

func getBlobSource() store.BlobStore {
	var blobSource store.BlobStore

	if upstreamReflector != "" {
		switch upstreamProtocol {
		case "tcp":
			blobSource = peer.NewStore(peer.StoreOpts{
				Address: upstreamReflector,
				Timeout: 30 * time.Second,
			})
		case "http3":
			blobSource = http3.NewStore(http3.StoreOpts{
				Address: upstreamReflector,
				Timeout: 30 * time.Second,
			})
		case "http":
			blobSource = store.NewHttpStore(upstreamReflector, edgeToken)
		default:
			Logger.Fatalf("protocol is not recognized: %s", upstreamProtocol)
		}

	} else if cloudFrontEndpoint != "" {
		blobSource = store.NewCloudFrontROStore(cloudFrontEndpoint)
	} else {
		Logger.Fatal("one of [--upstream-reflector|--cloudfront-endpoint] is required")
	}

	diskCacheMaxSize, diskCachePath := diskCacheParams() //TODO: use reflector code instead of code duplication
	//we are tracking blobs in memory with a 1 byte long boolean, which means that for each 2MB (a blob) we need 1Byte
	// so if the underlying cache holds 10MB, 10MB/2MB=5Bytes which is also the exact count of objects to restore on startup
	realCacheSize := float64(diskCacheMaxSize) / float64(stream.MaxBlobSize)
	if diskCacheMaxSize > 0 {
		err := os.MkdirAll(diskCachePath, os.ModePerm)
		if err != nil {
			Logger.Fatal(err)
		}
		blobSource = store.NewCachingStore(
			"player",
			blobSource,
			store.NewGcacheStore("player", store.NewDiskStore(diskCachePath, 2), int(realCacheSize), store.LRU),
		)
	}

	return blobSource
}

func diskCacheParams() (int, string) {
	l := Logger

	if diskCacheDir == "" {
		return 0, ""
	}

	path := diskCacheDir
	if len(path) == 0 || path[0] != '/' {
		l.Fatal("--disk-cache-dir must start with '/'")
	}

	var maxSize datasize.ByteSize
	err := maxSize.UnmarshalText([]byte(diskCacheSize))
	if err != nil {
		l.Fatal(err)
	}
	if maxSize <= 0 {
		l.Fatal("--disk-cache-size must be more than 0")
	}

	return int(maxSize), path
}

func initLogger() {
	logLevel := logrus.InfoLevel
	if verboseOutput {
		logLevel = logrus.DebugLevel
	}
	logger.ConfigureDefaults(logLevel)
	Logger.Infof("initializing %v\n", version.FullName())
	logger.ConfigureSentry(version.Version(), logger.EnvProd)
}

func initPubkey() {
	l := Logger

	r, err := http.Get(paidPubKey)
	if err != nil {
		l.Fatal(err)
	}
	rawKey, err := io.ReadAll(r.Body)
	if err != nil {
		l.Fatal(err)
	}
	err = paid.InitPubKey(rawKey)
	if err != nil {
		l.Fatal(err)
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		Logger.Fatalf("error: %v\n", err)
	}
}
