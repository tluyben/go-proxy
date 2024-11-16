# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=proxy-server
BINARY_UNIX=$(BINARY_NAME)_unix

all: test build

build:
	$(GOBUILD) -o $(BINARY_NAME) -v

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...
	./$(BINARY_NAME)

deps:
	$(GOGET) gopkg.in/yaml.v2

# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v

docker-build:
	docker build -t $(BINARY_NAME):latest .

linux-amd64:
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-linux-amd64 -v

deploy-s3: linux-amd64
	@echo "Starting deploy to S3..."
	@which rclone || (echo "rclone not found" && exit 1)
	@echo "Rclone found, proceeding with upload..."
	bash -c 'set -ex; \
		source .env; \
		rclone --s3-provider Other \
		--s3-endpoint=$$S3_ENDPOINT \
		--s3-access-key-id=$$S3_ACCESS_KEY \
		--s3-secret-access-key=$$S3_SECRET_KEY \
		moveto ./$(BINARY_NAME)-linux-amd64 :s3:binaries/$(BINARY_NAME)'

.PHONY: all build test clean run deps build-linux docker-build linux-amd64 deploy-s3
