TEST?=$$(go list ./... | grep -v 'vendor')
HOSTNAME=registry.terraform.io
NAMESPACE=cockroachdb
NAME=cockroach
BINARY=terraform-provider-${NAME}
VERSION=1.6.0
OS := $(shell uname | tr A-Z a-z)
ARCH := $(shell uname -m | sed 's/x86_64/amd64/')
OS_ARCH := $(OS)_$(ARCH)

default: install

build:
	go build -o ${BINARY}

# Generate Terraform Provider docs.
generate:
	go generate

update-sdk:
	go get github.com/cockroachdb/cockroach-cloud-sdk-go@api-cloud-20
	go generate ./mock

release:
	GOOS=darwin GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_darwin_amd64
	GOOS=freebsd GOARCH=386 go build -o ./bin/${BINARY}_${VERSION}_freebsd_386
	GOOS=freebsd GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_freebsd_amd64
	GOOS=freebsd GOARCH=arm go build -o ./bin/${BINARY}_${VERSION}_freebsd_arm
	GOOS=linux GOARCH=386 go build -o ./bin/${BINARY}_${VERSION}_linux_386
	GOOS=linux GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_linux_amd64
	GOOS=linux GOARCH=arm go build -o ./bin/${BINARY}_${VERSION}_linux_arm
	GOOS=openbsd GOARCH=386 go build -o ./bin/${BINARY}_${VERSION}_openbsd_386
	GOOS=openbsd GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_openbsd_amd64
	GOOS=solaris GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_solaris_amd64
	GOOS=windows GOARCH=386 go build -o ./bin/${BINARY}_${VERSION}_windows_386
	GOOS=windows GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_windows_amd64

# Use this to install a development binary to your local machine, in the TF
# provider cache. Note that if this directory is present, using an "official"
# version of the provider is disabled. Use "make clean" to reset.
install: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}
	mv ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}

clean:
	rm -rf ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/
	go clean -testcache -i -x

test:
	go test ./... -v $(TESTARGS) -timeout 5m

testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m -parallel 100
