all: build-cli build-service

build-cli:
	cd cli && go build -v && mv cli ../oms-cli

build-service:
	cd service && go build -v && mv service ../oms-service

test: test-cli test-service

test-cli:
	# -count=1 to disable caching test results
	go test -count=1 -v ./cli/...

test-service:
	go test -count=1 -v ./service/...

format:
	go fmt ./...

lint: install-build-deps
	golangci-lint run

install-build-deps:
ifeq (, $(shell which mockery))
	go install github.com/vektra/mockery/v3@v3.2.1
endif
ifeq (, $(shell which golangci-lint))
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.2
endif

generate: install-build-deps
	go generate ./...
