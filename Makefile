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
	goreleaser --rm-dist

snapshot:
	goreleaser --snapshot --rm-dist

version := $(shell git describe --abbrev=0 --tags|sed 's/v//')
.PHONY: image
image:
	docker build -t lbry/lbrytv-player:$(version) -t lbry/lbrytv-player:latest .

version := $(shell git describe --abbrev=0 --tags|sed 's/v//')
.PHONY: publish_image
publish_image:
	docker push lbry/lbrytv-player:$(version)
	docker push lbry/lbrytv-player:latest

tag := $(shell git describe --abbrev=0 --tags)
.PHONY: retag
retag:
	@echo "Re-setting tag $(tag) to the current commit"
	git push origin :$(tag)
	git tag -d $(tag)
	git tag $(tag)
