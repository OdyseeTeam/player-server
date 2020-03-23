package cmd

import (
	"fmt"
	"time"

	"github.com/lbryio/lbrytv-player/internal/version"
	"github.com/lbryio/lbrytv-player/pkg/app"
	"github.com/lbryio/lbrytv-player/pkg/logger"
	"github.com/lbryio/lbrytv-player/player"

	"github.com/c2h5oh/datasize"
	"github.com/spf13/cobra"
)

var Logger = logger.GetLogger()

var (
	bindAddress      string
	cachePath        string
	cacheSize        string
	enablePrefetch   bool
	enableProfile    bool
	useQuic    bool
	reflectorAddress string
	reflectorTimeout int
	lbrynetAddress   string

	cacheSizeBytes datasize.ByteSize

	rootCmd = &cobra.Command{
		Use:     "lbrytv_player",
		Short:   "media server for lbrytv",
		Version: version.FullName(),
		Run: func(cmd *cobra.Command, args []string) {
			Logger.Infof("initializing %v\n", version.FullName())
			logger.ConfigureSentry(version.Version(), logger.EnvProd)
			defer logger.Flush()

			err := cacheSizeBytes.UnmarshalText([]byte(cacheSize))
			if err != nil {
				Logger.Fatalf("error: %v\n", err)
			}

			pOpts := &player.Opts{
				EnableL2Cache:    true,
				CacheSize:        cacheSizeBytes,
				CachePath:        cachePath,
				EnablePrefetch:   enablePrefetch,
				ReflectorAddress: reflectorAddress,
				ReflectorTimeout: time.Second * time.Duration(reflectorTimeout),
				LbrynetAddress:   lbrynetAddress,
				UseQuicProtocol: useQuic,
			}
			if pOpts.CacheSize == 0 {
				pOpts.EnableL2Cache = false
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
	rootCmd.Flags().IntVar(&reflectorTimeout, "reflector_timeout", 30, "reflector timeout in seconds")
	rootCmd.Flags().StringVar(&lbrynetAddress, "lbrynet", "http://localhost:5279/", "lbrynet server URL")
	rootCmd.Flags().BoolVar(&enablePrefetch, "prefetch", true, "enable prefetch for blobs")
	rootCmd.Flags().BoolVar(&enableProfile, "profile", false, fmt.Sprintf("enable profiling server at %v", player.ProfileRoutePath))
	rootCmd.Flags().BoolVar(&useQuic, "use-quic", false, fmt.Sprintf("use the QUIC protocol instead of TCP"))
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		Logger.Fatalf("error: %v\n", err)
	}
}
