# Media server for Odysee

![Tests](https://github.com/OdyseeTeam/player-server/actions/workflows/pipeline.yml/badge.svg) [![Coverage Status](https://coveralls.io/repos/github/OdyseeTeam/player-server/badge.svg?branch=master)](https://coveralls.io/github/OdyseeTeam/player-server?branch=master)

# Development

To run tests:

```
make prepare_test
make test
```

# Usage

`odysee_player` requires a lbrynet-compatible API endpoint and MySQL (for decrypted cache metadata). Provide a `config.json` (see `config.example.json`) and apply the schema from `init.sql` to the MySQL database.

```
go run . \
      --disk-cache-dir=/tmp/player_cache \
      --disk-cache-size=800GB \
      --upstream-reflector=eu-p2.lbryplayer.xyz:5569 \
      --upstream-protocol=http \
      --bind=0.0.0.0:8081 \
      --prefetch=true \
      --hot-cache-size=6GB \
      --profile=true \
      --config-username=lbry \
      --config-password=beameriscool \
      --throttle-scale=3.0 \
      --throttle-enabled=false \
      --transcoder-video-path=/tmp/transcoded_cache \
      --transcoder-video-size=800GB \
      --transcoder-addr=https://root.transcoder.odysee.com \
      --transcoder-remote-server=https://cache-us.transcoder.odysee.com/t-na
```

Run `odysee_player -h` to see the full list of flags and options.

### Some flags explained

`cloudfront-endpoint` and `upstream-reflector` are mutually exclusive: the player can either pull blobs from an existing reflector or from a CDN endpoint. Only specify one.

- example for `cloudfront-endpoint`: http://XXXXXXXXXX.cloudfront.net/
- example for `upstream-reflector`: reflector.lbry.com:5568

`upstream-protocol` supports `tcp`, `http3`, and `http` for fetching blobs from an upstream reflector.

`lbrynet` sets the API endpoint used to resolve claims (defaults to Odysee backend).

`paid_pubkey` and `edge-token` are used for paid streams; `paid_pubkey` is fetched at startup, and `edge-token` is passed upstream for authorization.

`disk-cache-dir` and `disk-cache-size` refer to the location and size where encrypted blobs are stored locally. Access is then regulated using Least Frequently Accessed (with Dynamic Aging) as eviction strategy.

`hot-cache-size` refers to the size of the in memory cache where unencrypted blobs are stored. Blobs are evicted using LRU as strategy.

`prefetch` and `prefetch-count` can help reduce buffering by downloading blobs to the player in advance so that they're ready when they'll be requested by the client in the near future.

`throttle-enabled` and `throttle-scale` allow for limiting the outbound bandwidth on a per stream resolution. This helps ensure that no single client can saturate the uplink pipe of the server.

`transcoder-video-path` and `transcoder-video-size` are similar to the disk cache flags; they regulate the location and size of transcoded videos retrieved from `transcoder-addr`.

### Config file (config.json)

The decrypted cache uses a gody-cdn configuration file and a MySQL database. Copy `config.example.json` to `config.json`, update `local_db` and `disk_cache`, and apply `init.sql` to the MySQL database.

### Operational endpoints

- `/metrics` exposes Prometheus metrics.
- `/config/throttle` (POST, basic auth) updates `enabled` and/or `scale` at runtime.
- `/config/blacklist` (POST, basic auth) reloads `blacklist.json`.
- `/superdebug/pprof` is available when `--profile` is enabled.

### Firewall / blacklist

Create a `blacklist.json` file in the working directory to block IPs or ASNs:

```
{
  "blacklisted_asn": [12345],
  "blacklisted_ips": ["203.0.113.0/24", "198.51.100.1/32"]
}
```

Set `MAXMIND_KEY` to enable ASN lookups via GeoLite2-ASN. Without it, ASN blocking is skipped.

### Environment variables

- `SENTRY_DSN` enables Sentry error reporting.
- `PLAYER_NAME` sets the server name used in logs and Sentry.
- `MAXMIND_KEY` enables GeoLite2-ASN downloads for firewall ASN checks.

## Running with Docker

The primary way player server is intended to run is in a docker environment managed by `docker-compose`. To launch and start serving:

```
cp config.example.json config.json
docker-compose up -d
```

# Build and release

Tag a new release (CalVer) before building/publishing images:

```
git tag v21.3.4  # March 2021, version 4
git push origin v21.3.4
```

Build the binary and container image:

```
make linux
make image
```

Publish images:

```
make publish_image
```

Check [Makefile](./Makefile) for more details.

## License

This project is MIT licensed. For the full license, see [LICENSE](LICENSE).

## Contact

The primary contact for this project is [@anbsky](https://github.com/anbsky) (andrey.beletsky@odysee.com).
