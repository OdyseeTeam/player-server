package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/lbryio/lbrytv-player/pkg/server"
	"github.com/lbryio/lbrytv-player/player"

	"github.com/c2h5oh/datasize"
	"github.com/spf13/cobra"
)

var (
	bindAddress      string
	cachePath        string
	cacheSize        string
	enablePrefetch   bool
	reflectorAddress string
	reflectorTimeout int
	lbrynetAddress   string

	cacheSizeBytes datasize.ByteSize

	rootCmd = &cobra.Command{
		Use:   "lbrytv_player",
		Short: "media server for lbrytv",
		Run: func(cmd *cobra.Command, args []string) {
			err := cacheSizeBytes.UnmarshalText([]byte(cacheSize))
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}

			pOpts := &player.Opts{
				EnableL2Cache:    true,
				CacheSize:        cacheSizeBytes,
				CachePath:        cachePath,
				EnablePrefetch:   enablePrefetch,
				ReflectorAddress: reflectorAddress,
				ReflectorTimeout: time.Second * time.Duration(reflectorTimeout),
				LbrynetAddress:   lbrynetAddress,
			}
			if pOpts.CacheSize == 0 {
				pOpts.EnableL2Cache = false
			}
			p := player.NewPlayer(pOpts)

			s := server.NewServer(server.Opts{
				Address: bindAddress,
			})

			player.InstallPlayerRoutes(s.Router, p)
			player.InstallMetricsRoutes(s.Router)

			s.Start()
			s.ServeUntilShutdown()
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
	rootCmd.Flags().BoolVar(&enablePrefetch, "enable_prefetch", true, "enable prefetch for blobs")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
