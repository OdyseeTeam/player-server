package cmd

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lbryio/lbrytv-player/internal/metrics"
	"github.com/lbryio/lbrytv-player/internal/version"
	"github.com/lbryio/lbrytv-player/pkg/app"
	"github.com/lbryio/lbrytv-player/pkg/logger"
	"github.com/lbryio/lbrytv-player/pkg/paid"
	"github.com/lbryio/lbrytv-player/player"

	"github.com/lbryio/lbry.go/v2/stream"
	"github.com/lbryio/reflector.go/peer/http3"
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
	lbrynetAddress string
	paidPubKey     string

	upstreamReflector  string
	cloudFrontEndpoint string
	diskCacheConfig    string
	hotCacheSize       string

	rootCmd = &cobra.Command{
		Use:     "lbrytv_player",
		Short:   "media server for lbrytv",
		Version: version.FullName(),
		Run:     run,
	}
)

func init() {
	rootCmd.Flags().StringVar(&bindAddress, "bind", "0.0.0.0:8080", "address to bind HTTP server to")
	rootCmd.Flags().StringVar(&lbrynetAddress, "lbrynet", "http://localhost:5279/", "lbrynet server URL")
	rootCmd.Flags().StringVar(&paidPubKey, "paid_pubkey", "https://api.lbry.tv/api/v1/paid/pubkey", "pubkey for playing paid content")

	rootCmd.Flags().BoolVar(&enablePrefetch, "prefetch", false, "enable prefetch for blobs")
	rootCmd.Flags().BoolVar(&enableProfile, "profile", false, fmt.Sprintf("enable profiling server at %v", player.ProfileRoutePath))
	rootCmd.Flags().BoolVar(&verboseOutput, "verbose", false, fmt.Sprintf("enable verbose logging"))

	rootCmd.Flags().StringVar(&upstreamReflector, "upstream-reflector", "", "host:port of a reflector server where blobs are fetched from")
	rootCmd.Flags().StringVar(&cloudFrontEndpoint, "cloudfront-endpoint", "", "CloudFront edge endpoint for standard HTTP retrieval")
	rootCmd.Flags().StringVar(&diskCacheConfig, "disk-cache", "",
		"enable disk cache, setting max size and path where to store blobs. format is 'MAX_BLOBS:CACHE_PATH'. MAX_BLOBS can be 16GB, 500MB, etc.")
	rootCmd.Flags().StringVar(&hotCacheSize, "hot-cache-size", "", "enable hot cache for decrypted blobs and set max size: 16GB, 500MB, etc")
}

func run(cmd *cobra.Command, args []string) {
	initLogger()
	defer logger.Flush()

	initPubkey()

	blobSource := getBlobSource()

	p := player.NewPlayer(getHotCache(blobSource), lbrynetAddress)
	p.SetPrefech(enablePrefetch)

	a := app.New(app.Opts{Address: bindAddress, BlobStore: blobSource})

	player.InstallPlayerRoutes(a.Router, p)
	metrics.InstallRoute(a.Router)
	if enableProfile {
		player.InstallProfilingRoutes(a.Router)
	}

	a.Start()
	a.ServeUntilShutdown()
}

func getHotCache(blobSource store.BlobStore) *player.HotCache {
	var hotCacheBytes datasize.ByteSize
	err := hotCacheBytes.UnmarshalText([]byte(hotCacheSize))
	if err != nil {
		Logger.Fatal(err)
	}
	if hotCacheBytes <= 0 {
		Logger.Fatal("hot cache size must be greater than 0. if you want to disable hot cache, you'll have to do a bit of coding")
	}

	avgSDBlobSize := 1000      // JUST A GUESS
	fractionForSDBlobs := 0.10 // 10% of cache space for sd blobs

	spaceForSDBlobs := int(float64(hotCacheBytes.Bytes()) * fractionForSDBlobs)
	spaceForChunks := int(hotCacheBytes.Bytes()) - spaceForSDBlobs

	return player.NewHotCache(blobSource, spaceForChunks/player.MaxChunkSize, spaceForSDBlobs/avgSDBlobSize)
}

func getBlobSource() store.BlobStore {
	var blobSource store.BlobStore

	if upstreamReflector != "" {
		blobSource = http3.NewStore(http3.StoreOpts{
			Address: upstreamReflector,
			Timeout: 30 * time.Second,
		})
	} else if cloudFrontEndpoint != "" {
		blobSource = store.NewCloudFrontROStore(cloudFrontEndpoint)
	} else {
		Logger.Fatal("one of [--proxy-address|--cloudfront-endpoint] is required")
	}

	diskCacheMaxSize, diskCachePath := diskCacheParams()
	if diskCacheMaxSize > 0 {
		err := os.MkdirAll(diskCachePath, os.ModePerm)
		if err != nil {
			Logger.Fatal(err)
		}
		blobSource = store.NewCachingStore(
			"player",
			blobSource,
			store.NewLRUStore("player", store.NewDiskStore(diskCachePath, 2), diskCacheMaxSize/stream.MaxBlobSize),
		)
	}

	return blobSource
}

func diskCacheParams() (int, string) {
	l := Logger

	if diskCacheConfig == "" {
		return 0, ""
	}

	parts := strings.Split(diskCacheConfig, ":")
	if len(parts) != 2 {
		l.Fatalf("--disk-cache must be a number, followed by ':', followed by a string")
	}

	var maxSize datasize.ByteSize
	err := maxSize.UnmarshalText([]byte(parts[0]))
	if err != nil {
		l.Fatal(err)
	}
	if maxSize <= 0 {
		l.Fatal("--disk-cache max size must be more than 0")
	}

	path := parts[1]
	if len(path) == 0 || path[0] != '/' {
		l.Fatal("--disk-cache path must start with '/'")
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
	rawKey, err := ioutil.ReadAll(r.Body)
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
