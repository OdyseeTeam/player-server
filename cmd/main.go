package cmd

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/lbryio/lbrytv-player/internal/version"
	"github.com/lbryio/lbrytv-player/pkg/app"
	"github.com/lbryio/lbrytv-player/pkg/logger"
	"github.com/lbryio/lbrytv-player/pkg/paid"
	"github.com/lbryio/lbrytv-player/player"

	"github.com/c2h5oh/datasize"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	bindAddress       string
	cachePath         string
	cacheSize         string
	enablePrefetch    bool
	enableProfile     bool
	reflectorProtocol string
	verboseOutput     bool
	reflectorAddress  string
	reflectorTimeout  int
	lbrynetAddress    string
	paidPubKey        string
	hotCacheSize      int

	cacheSizeBytes datasize.ByteSize

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

			err := cacheSizeBytes.UnmarshalText([]byte(cacheSize))
			if err != nil {
				l.Fatalf("error: %v\n", err)
			}

			var cache player.ChunkCache
			if cacheSizeBytes > 0 {
				cache, err = player.InitLRUCache(&player.LRUCacheOpts{Path: cachePath, Size: uint64(cacheSizeBytes)})
				if err != nil {
					l.Fatalf("cannot initialize cache: %v\n", err)
				}
			}
			pOpts := &player.Opts{
				LocalCache:        cache,
				EnablePrefetch:    enablePrefetch,
				ReflectorAddress:  reflectorAddress,
				ReflectorTimeout:  time.Second * time.Duration(reflectorTimeout),
				LbrynetAddress:    lbrynetAddress,
				ReflectorProtocol: reflectorProtocol,
				HotCacheSize:      hotCacheSize,
			}

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

			p := player.NewPlayer(pOpts)

			a := app.New(app.Opts{
				Address: bindAddress,
			})

			player.InstallPlayerRoutes(a.Router, p)
			player.InstallMetricsRoutes(a.Router)
			if enableProfile {
				player.InstallProfilingRoutes(a.Router)
			}

			a.Start()
			a.ServeUntilShutdown()
		},
	}
)

func init() {
	rootCmd.Flags().StringVar(&cachePath, "cache_path", "/tmp/player_cache", "cache directory path (will be created if does not exist)")
	rootCmd.Flags().StringVar(&cacheSize, "cache_size", "", "cache size: 16GB, 500MB and so on, set to 0 to disable")
	rootCmd.Flags().StringVar(&bindAddress, "bind", "0.0.0.0:8080", "address to bind HTTP server to")
	rootCmd.Flags().StringVar(&reflectorAddress, "reflector", "", "reflector address (with port)")
	rootCmd.Flags().StringVar(&paidPubKey, "paid_pubkey", "https://api.lbry.tv/api/v1/paid/pubkey", "pubkey for playing paid content")
	rootCmd.Flags().IntVar(&reflectorTimeout, "reflector_timeout", 30, "reflector timeout in seconds")
	rootCmd.Flags().IntVar(&hotCacheSize, "ram_cache_size", 4096, "ram cache size in MB")
	rootCmd.Flags().StringVar(&lbrynetAddress, "lbrynet", "http://localhost:5279/", "lbrynet server URL")
	rootCmd.Flags().BoolVar(&enablePrefetch, "prefetch", true, "enable prefetch for blobs")
	rootCmd.Flags().BoolVar(&enableProfile, "profile", false, fmt.Sprintf("enable profiling server at %v", player.ProfileRoutePath))
	rootCmd.Flags().StringVar(&reflectorProtocol, "reflector_protocol", "http3", fmt.Sprintf("which protocol to use to fetch the data from reflector (tcp/http3)"))
	rootCmd.Flags().BoolVar(&verboseOutput, "verbose", false, fmt.Sprintf("enable verbose logging"))
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.GetLogger().Fatalf("error: %v\n", err)
	}
}
