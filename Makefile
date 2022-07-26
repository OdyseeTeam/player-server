version := $(shell git describe --tags)



.PHONY: prepare_test
prepare_test:
	curl https://raw.githubusercontent.com/OdyseeTeam/gody-cdn/master/db-init.sql -o init.sql
	docker-compose up -d mysql lbrynet

.PHONY: test
test:
	go test -cover ./...

.PHONY: test_ci
test_ci:
	scripts/wait_for_wallet.sh
	go install golang.org/x/tools/cmd/cover@latest
	go install github.com/mattn/goveralls@latest
	go install github.com/jandelgado/gcov2lcov@latest
	go test -covermode=count -coverprofile=coverage.out ./...
	gcov2lcov -infile=coverage.out -outfile=coverage.lcov

linux:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -o dist/linux_amd64/odysee_player -ldflags "-s -w -X github.com/OdyseeTeam/player-server/internal/version.version=$(version)"

macos:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=darwin go build -o dist/darwin_amd64/odysee_player -ldflags "-s -w -X github.com/OdyseeTeam/player-server/internal/version.version=$(version)"

version := $(shell git describe --abbrev=0 --tags|sed 's/v//')
cur_branch := $(shell git rev-parse --abbrev-ref HEAD)
.PHONY: image
image:
	docker buildx build -t odyseeteam/player-server:$(version) -t odyseeteam/player-server:latest -t odyseeteam/player-server:$(cur_branch) --platform linux/amd64 .

version := $(shell git describe --abbrev=0 --tags|sed 's/v//')
.PHONY: publish_image
publish_image:
	docker push odyseeteam/player-server:$(version)
	docker tag odyseeteam/player-server:$(version) odyseeteam/player-server:latest odyseeteam/player-server:$(cur_branch)
	docker push odyseeteam/player-server:latest

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
