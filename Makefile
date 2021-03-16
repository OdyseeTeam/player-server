VERSION := $(shell git describe --tags)

.PHONY: test
test:
	go test -cover ./...

.PHONY: test_circleci
test_circleci:
	scripts/wait_for_wallet.sh
	go get golang.org/x/tools/cmd/cover
	go get github.com/mattn/goveralls
	go test -covermode=count -coverprofile=coverage.out ./...
	goveralls -coverprofile=coverage.out -service=circle-ci -repotoken $(COVERALLS_TOKEN)

release:
	goreleaser --rm-dist --skip-publish

snapshot:
	goreleaser --rm-dist --snapshot

linux:
	GOARCH=amd64 GOOS=linux go build -o dist/lbrytv_player -ldflags "-s -w -X github.com/lbryio/lbrytv-player/internal/version.version=$(VERSION)"

macos:
	GOARCH=amd64 GOOS=darwin go build -o dist/lbrytv_player -ldflags "-s -w -X github.com/lbryio/lbrytv-player/internal/version.version=$(VERSION)"

version := $(shell git describe --abbrev=0 --tags|sed 's/v//')
.PHONY: image
image:
	docker build -t lbry/lbrytv-player:$(version) -t lbry/lbrytv-player:latest .

version := $(shell git describe --abbrev=0 --tags|sed 's/v//')
.PHONY: publish_image
publish_image:
	docker push lbry/lbrytv-player:$(version)
	docker tag lbry/lbrytv-player:$(version) lbry/lbrytv-player:latest
	docker push lbry/lbrytv-player:latest

tag := $(shell git describe --abbrev=0 --tags)
.PHONY: retag
retag:
	@echo "Re-setting tag $(tag) to the current commit"
	git push origin :$(tag)
	git tag -d $(tag)
	git tag $(tag)


.PHONY: dev hotdev
dev:
	go run . --upstream-reflector=reflector.lbry.com:5568 --verbose --hot-cache-size=50M
hotdev:
	reflex --decoration=none --start-service=true --regex='\.go$$' make dev
