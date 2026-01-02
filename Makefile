all: build-cli build-service

build-cli:
	cd cli && go build -v && mv cli ../oms-cli

build-service:
	cd service && go build -v && mv service ../oms-service

test:
	go test -count=1 -v ./...

test-integration:
	# Run integration tests with build tag
	go test -count=1 -v -tags=integration ./cli/...

test-service:
	go test -count=1 -v ./service/...

format:
	go fmt ./...

lint: install-build-deps
	go tool golangci-lint run

install-build-deps:
ifeq (, $(shell which go-licenses))
	go install github.com/google/go-licenses@v1.6.0
endif
ifeq (, $(shell which copywrite))
	go install github.com/hashicorp/copywrite@v0.22.0
endif

generate: install-build-deps
	go tool mockery
	go generate ./...

VERSION ?= "0.0.0"
release-local: install-build-deps
	rm -rf dist
	/bin/bash -c "go tool goreleaser --snapshot --skip=validate,announce,publish -f <(sed s/{{.Version}}/$(VERSION)/g < .goreleaser.yaml)"

.PHONY: docs
docs:
	rm -rf docs
	mkdir docs
	go run -ldflags="-X 'github.com/codesphere-cloud/oms/internal/version.binName=oms-cli'" hack/gendocs/main.go
	cp docs/oms-cli.md docs/README.md

generate-license: generate
	go-licenses report --template .NOTICE.template  ./... > NOTICE
	copywrite headers apply

run-lima:
	limactl start ./hack/lima-oms.yaml

stop-lima:
	limactl stop lima-oms
	limactl delete lima-oms
