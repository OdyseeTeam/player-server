# Media server for Odysee

![Tests](https://github.com/OdyseeTeam/player-server/actions/workflows/pipeline.yml/badge.svg) [![Coverage Status](https://coveralls.io/repos/github/OdyseeTeam/player-server/badge.svg?branch=master)](https://coveralls.io/github/OdyseeTeam/player-server?branch=master)


# Usage

Player only requires a working instance of [lbrynet](https://github.com/lbryio/lbry) running nearby.

Run `odysee_player -h` to see flags and options.
```
media server for odysee.com

Usage:
  odysee_player [flags]

Flags:
      --bind string                       address to bind HTTP server to (default "0.0.0.0:8080")
      --cloudfront-endpoint string        CloudFront edge endpoint for standard HTTP retrieval
      --config-password string            Password to access the config endpoint with (default "lbry")
      --config-username string            Username to access the config endpoint with (default "lbry")
      --disk-cache-dir string             enable disk cache, storing blobs in dir
      --disk-cache-size string            max size of disk cache: 16GB, 500MB, etc. (default "100MB")
  -h, --help                              help for odysee_player
      --hot-cache-size string             max size for in-memory cache: 16GB, 500MB, etc (default "50MB")
      --http-read-timeout uint            read timeout for http requests (seconds) (default 10)
      --http-stream-write-timeout uint    write timeout for stream http requests (seconds) (default 86400)
      --http-write-timeout uint           write timeout for http requests (seconds) (default 15)
      --lbrynet string                    lbrynet server URL (default "http://localhost:5279/")
      --paid_pubkey string                pubkey for playing paid content (default "https://api.na-backend.odysee.com/api/v1/paid/pubkey")
      --prefetch                          enable prefetch for blobs
      --prefetch-count uint               how many blobs to retrieve from origin in advance (default 2)
      --profile                           enable profiling server at /superdebug/pprof
      --throttle-enabled                  Enables throttling (default true)
      --throttle-scale float              Throttle scale to rate limit in MB/s, only the 1.2 in 1.2MB/s (default 1.5)
      --transcoder-addr string            transcoder API address
      --transcoder-remote-server string   remote transcoder storage server URL
      --transcoder-video-path string      path to store transcoded videos
      --transcoder-video-size string      max size of transcoder video storage (default "200GB")
      --upstream-protocol string          protocol used to fetch blobs from another upstream reflector server (tcp/http3/http) (default "http")
      --upstream-reflector string         host:port of a reflector server where blobs are fetched from
      --verbose                           enable verbose logging
  -v, --version                           version for odysee_player
```

### flags in details

`cloudfront-endpoint` and `upstream-reflector` are mutually exclusive: the player can either pull blobs from an existing reflector or from a CDN endpoint. Only specify one.

- example for `cloudfront-endpoint`: http://XXXXXXXXXX.cloudfront.net/
- example for `upstream-reflector`: reflector.lbry.com:5568

`disk-cache-dir` and `disk-cache-size` refer to the location and size where encrypted blobs are stored locally. Access is then regulated using Least Frequently Accessed (with Dynamic Aging) as eviction strategy.

`hot-cache-size` refers to the size of the in memory cache where unencrypted blobs are stored. Blobs are evicted using LRU as strategy.

`prefetch` and `prefetch-count` can help reduce buffering by downloading blobs to the player in advance so that they're ready when they'll be requested by the client in the near future.

`throttle-enabled` and `throttle-scale` allow for limiting the outbound bandwidth on a per stream resolution. This helps ensure that no single client can saturate the uplink pipe of the server.

`transcoder-video-path` `transcoder-video-size` similarly to the disk cache flags, these regulate the location and size of the transcoded videos that are retrieved from `transcoder-addr`

## Running with Docker

The primary way player server is intended to run is in a docker environment managed by `docker-compose`. To launch and start serving:

```
docker-compose up -d
```

# Releasing

Releases are built and packed into docker images automatically off `master` branch by Circle CI. Approximate `Makefile` commands that are used:

```
make linux
make image
docker push odyseeteam/player-server:21.3.4
docker push odyseeteam/player-server:latest
```

You need to tag your commit with a proper CalVer tag. Example:

```
git tag v21.3.4  # March 2021, version 4
```

Check [Makefile](./Makefile) for more details.

## License

This project is MIT licensed. For the full license, see [LICENSE](LICENSE).

## Contact

The primary contact for this project is [@andybeletsky](https://github.com/andybeletsky) (andrey.beletsky@odysee.com).
