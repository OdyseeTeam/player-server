# Media server for LBRYtv

[![CircleCI](https://img.shields.io/circleci/project/github/lbryio/lbrytv-player/master.svg)](https://circleci.com/gh/lbryio/lbrytv-player/tree/master) [![Coverage](https://img.shields.io/coveralls/github/lbryio/lbrytv-player.svg)](https://coveralls.io/github/lbryio/lbrytv-player)


# Usage

LBRYtv player only requires a working instance of [lbrynet](https://github.com/lbryio/lbry) running nearby.

Run `lbrytv_player -h` to see flags and options.

## Running with Docker

The primary way lbrytv-player is intended to run is in a docker environment managed by `docker-compose`. To launch and start serving:

```
docker-compose up -d
```

# Releasing

Releases are built and packed into docker images automatically off the master branch by Circle CI. You need to tag your commit with a proper semver tag.

Both release binary and docker image are built using goreleaser. Check [Makefile](./Makefile) and [goreleaser config](./.goreleaser.yml) for more details.

## Contributing

Contributions to this project are welcome, encouraged, and compensated. For more details, see [lbry.io/faq/contributing](https://lbry.io/faq/contributing).

Please ensure that your code builds and automated tests run successfully before pushing your branch. You must `go fmt` your code before you commit it, or the build will fail.

## License

This project is MIT licensed. For the full license, see [LICENSE](LICENSE).


## Security

We take security seriously. Please contact security@lbry.io regarding any issues you may encounter.
Our PGP key is [here](https://keybase.io/lbry/key.asc) if you need it.


## Contact

The primary contact for this project is [@sayplastic](https://github.com/sayplastic) (andrey@lbry.com).

