# Media server for Odysee

![Tests](https://github.com/OdyseeTeam/player-server/actions/workflows/pipeline.yml/badge.svg) [![Coverage Status](https://coveralls.io/repos/github/OdyseeTeam/player-server/badge.svg?branch=master)](https://coveralls.io/github/OdyseeTeam/player-server?branch=master)

# Development

To run tests:

```
make prepare_test
make test
```

# Usage

`player-server` requires lbry SDK and mysql.

```
go run .\
      --disk-cache-dir=/tmp/player_cache\
      --disk-cache-size=800GB\
      --upstream-reflector=eu-p2.lbryplayer.xyz:5569\
      --upstream-protocol=http\
      --bind=0.0.0.0:8081\
      --prefetch=true\
      --hot-cache-size=6GB\
      --profile=true\
      --config-username=lbry\
      --config-password=beameriscool\
      --throttle-scale=3.0\
      --throttle-enabled=false\
      --transcoder-video-path=/tmp/transcoded_cache\
      --transcoder-video-size=800GB\
      --transcoder-addr=https://root.transcoder.odysee.com \
      --transcoder-remote-server=https://cache-us.transcoder.odysee.com/t-na
```

Run `odysee_player -h` to see the full list of flags and options.

### Some flags explained

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

Releases are built and packed into docker images automatically off `master` branch. Approximate `Makefile` commands that are used:

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

The primary contact for this project is [@anbsky](https://github.com/anbsky) (andrey.beletsky@odysee.com).
