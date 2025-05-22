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
