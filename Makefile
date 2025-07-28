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

run-ssh-container:
	docker run -v ./key.pub:/root/key.pub -p 10022:22 --rm -ti --entrypoint="/bin/sh" --name codesphere-host \
		testcontainers/sshd:1.2.0 -c \
		"mkdir -p /root/.ssh && cp /root/key.pub /root/.ssh/authorized_keys && chmod 700 /root/.ssh && chmod 644 /root/.ssh/authorized_keys && /usr/sbin/sshd -D -o PermitRootLogin=yes -o AddressFamily=inet -o GatewayPorts=yes -o AllowAgentForwarding=yes -o AllowTcpForwarding=yes -o KexAlgorithms=+diffie-hellman-group1-sha1 -o HostkeyAlgorithms=+ssh-rsa"

stop-ssh-container:
	docker stop codesphere-host
