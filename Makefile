all: cicd-build

# Input: RACE
RACE ?= 0
export CGO_ENABLED ?= 0
ifeq ($(RACE), 1)
	GO_RACE := -race
	export CGO_ENABLED := 1
else
	GO_RACE :=
endif

# This is the list of all Go files in the project.
# We'll use this as the dependency list for all of the Go-based binaries.
ALL_GO_FILES := $(shell find ./ -name '*.go')

# Get the current directory
current_dir = $(shell pwd)

.PHONY: clean
clean:
	go clean

# Upgrades all third party dependencies
.PHONY: upgrade-deps
upgrade-deps:
# Upgrade transitive dependencies
	go get -u ./...
	go get -u all
	go get -u
	go mod tidy

# Runs unit tests
.PHONY: test
test:
	go vet ./...
	go test -cover $(GO_RACE) -parallel 10 ./...

.PHONY: staticcheck
staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck@v0.6.1
	staticcheck ./...

.PHONY: format
format:
	go fmt ./...

#
# CI/CD targets
#

# Build all binaries and packages and the website
# This is useful to run in the CICD pipeline to make sure everything builds correctly
# Run this concurrently: -j 6 or 8 should do the trick.
# The order of the build targets matters in order to start with the ones that take the longest
.PHONY: cicd-build
cicd-build:
	go build ./...

