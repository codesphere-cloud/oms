build-cli:
	go build -v -o ./bin/oms-cli ./cmd/cli

build-service:
	go build -v -o ./bin/oms-svc ./cmd/service

