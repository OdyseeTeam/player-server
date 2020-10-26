package cmd

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/lbryio/lbrytv-player/internal/metrics"
	"github.com/lbryio/lbrytv-player/internal/version"
	"github.com/lbryio/lbrytv-player/pkg/app"
	"github.com/lbryio/lbrytv-player/pkg/logger"
	"github.com/lbryio/lbrytv-player/pkg/paid"
	"github.com/lbryio/lbrytv-player/player"

	"github.com/lbryio/lbry.go/v2/stream"
	"github.com/lbryio/reflector.go/peer"
	"github.com/lbryio/reflector.go/peer/http3"
	"github.com/lbryio/reflector.go/store"

	"github.com/c2h5oh/datasize"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	bindAddress       string
	diskCachePath     string
	diskCacheSize     string
	hotCacheSize      string
	enablePrefetch    bool
	enableProfile     bool
	reflectorProtocol string
	verboseOutput     bool
	reflectorAddress  string
	reflectorTimeout  int
	lbrynetAddress    string
	paidPubKey        string

	rootCmd = &cobra.Command{
		Use:     "lbrytv_player",
		Short:   "media server for lbrytv",
		Version: version.FullName(),
		Run: func(cmd *cobra.Command, args []string) {
			logLevel := logrus.InfoLevel
			if verboseOutput {
				logLevel = logrus.DebugLevel
			}
			logger.ConfigureDefaults(logLevel)

			l := logger.GetLogger()
			l.Infof("initializing %v\n", version.FullName())
			logger.ConfigureSentry(version.Version(), logger.EnvProd)
			defer logger.Flush()

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

			p := player.NewPlayer(&player.Opts{
				BlobSource:     getBlobSource(),
				HotCache:       getHotCache(),
				EnablePrefetch: enablePrefetch,
				LbrynetAddress: lbrynetAddress,
			})

			a := app.New(app.Opts{
				Address: bindAddress,
			})

			player.InstallPlayerRoutes(a.Router, p)
			metrics.InstallRoute(a.Router)
			if enableProfile {
				player.InstallProfilingRoutes(a.Router)
			}

			a.Start()
			a.ServeUntilShutdown()
		},
	}
)

func getBlobSource() store.BlobStore {
	l := logger.GetLogger()

	var diskCacheBytes datasize.ByteSize
	err := diskCacheBytes.UnmarshalText([]byte(diskCacheSize))
	if err != nil {
		l.Fatal(err)
	}

	var blobSource store.BlobStore
	if reflectorProtocol == "http3" {
		blobSource = http3.NewStore(http3.StoreOpts{Address: reflectorAddress, Timeout: time.Second * time.Duration(reflectorTimeout)})
	} else {
		blobSource = peer.NewStore(peer.StoreOpts{Address: reflectorAddress, Timeout: time.Second * time.Duration(reflectorTimeout)})
	}

	if diskCacheBytes > 0 {
		blobSource = store.NewCachingStore(
			blobSource,
			store.NewLRUStore(store.NewDiskStore(diskCachePath, 2), int(diskCacheBytes.Bytes()/stream.MaxBlobSize)),
		)
	}

	return blobSource
}

func getHotCache() *player.HotCache {
	l := logger.GetLogger()

	var hotCacheBytes datasize.ByteSize
	err := hotCacheBytes.UnmarshalText([]byte(hotCacheSize))
	if err != nil {
		l.Fatal(err)
	}
	if hotCacheBytes <= 0 {
		return nil
	}

	return player.NewHotCache(int64(hotCacheBytes.Bytes()) / stream.MaxBlobSize)
}

func init() {
	rootCmd.Flags().StringVar(&bindAddress, "bind", "0.0.0.0:8080", "address to bind HTTP server to")

	rootCmd.Flags().StringVar(&diskCachePath, "disk_cache_path", "/tmp/player_cache", "cache directory path (will be created if does not exist)")
	rootCmd.Flags().StringVar(&diskCacheSize, "disk_cache_size", "", "disk cache size: 16GB, 500MB and so on, set to 0 to disable")
	rootCmd.Flags().StringVar(&hotCacheSize, "hot_cache_size", "", "hot cache size for decrypted blobs: 16GB, 500MB and so on, set to 0 to disable")

	rootCmd.Flags().StringVar(&reflectorAddress, "reflector", "reflector.lbry.com:5568", "reflector address (with port)")
	rootCmd.Flags().IntVar(&reflectorTimeout, "reflector_timeout", 30, "reflector timeout in seconds")
	rootCmd.Flags().StringVar(&reflectorProtocol, "reflector_protocol", "http3", fmt.Sprintf("which protocol to use to fetch the data from reflector (tcp/http3)"))

	rootCmd.Flags().StringVar(&lbrynetAddress, "lbrynet", "http://localhost:5279/", "lbrynet server URL")
	rootCmd.Flags().StringVar(&paidPubKey, "paid_pubkey", "https://api.lbry.tv/api/v1/paid/pubkey", "pubkey for playing paid content")

	rootCmd.Flags().BoolVar(&enablePrefetch, "prefetch", true, "enable prefetch for blobs")
	rootCmd.Flags().BoolVar(&enableProfile, "profile", false, fmt.Sprintf("enable profiling server at %v", player.ProfileRoutePath))
	rootCmd.Flags().BoolVar(&verboseOutput, "verbose", false, fmt.Sprintf("enable verbose logging"))
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.GetLogger().Fatalf("error: %v\n", err)
	}
}
