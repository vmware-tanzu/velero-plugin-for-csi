# Copyright 2019, 2020 the Velero contributors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# The binary to build (just the basename).
BIN ?= velero-plugin-for-csi

BUILD_IMAGE ?= golang:1.17-stretch

REGISTRY ?= velero
IMAGE_NAME ?= $(REGISTRY)/velero-plugin-for-csi
TAG ?= dev

IMAGE ?= $(IMAGE_NAME):$(TAG)

# Which architecture to build - see $(ALL_ARCH) for options.
# if the 'local' rule is being run, detect the ARCH from 'go env'
# if it wasn't specified by the caller.
local : ARCH ?= $(shell go env GOOS)-$(shell go env GOARCH)
ARCH ?= linux-amd64

platform_temp = $(subst -, ,$(ARCH))
GOOS = $(word 1, $(platform_temp))
GOARCH = $(word 2, $(platform_temp))

.PHONY: all
all: $(addprefix build-, $(BIN))

build-%:
	$(MAKE) --no-print-directory BIN=$* build

.PHONY: local
local: build-dirs
	GOOS=$(GOOS) \
	GOARCH=$(GOARCH) \
	BIN=$(BIN) \
	OUTPUT_DIR=$$(pwd)/_output/bin/$(GOOS)/$(GOARCH) \
	./hack/build.sh

build: _output/bin/$(GOOS)/$(GOARCH)/$(BIN)

_output/bin/$(GOOS)/$(GOARCH)/$(BIN): build-dirs
	@echo "building: $@"
	$(MAKE) shell CMD="-c '\
		GOOS=$(GOOS) \
		GOARCH=$(GOARCH) \
		BIN=$(BIN) \
		OUTPUT_DIR=/output/$(GOOS)/$(GOARCH) \
		./hack/build.sh'"

TTY := $(shell tty -s && echo "-t")

shell: build-dirs 
	@echo "running docker: $@"
	@docker run \
		-e GOFLAGS \
		-i $(TTY) \
		--rm \
		-u $$(id -u):$$(id -g) \
		-v "$$(pwd)/_output/bin:/output:delegated" \
		-v $$(pwd)/.go/pkg:/go/pkg \
		-v $$(pwd)/.go/src:/go/src \
		-v $$(pwd)/.go/std:/go/std \
		-v $$(pwd):/go/src/velero-plugin-for-csi \
		-v $$(pwd)/.go/std/$(GOOS)_$(GOARCH):/usr/local/go/pkg/$(GOOS)_$(GOARCH)_static \
		-v "$$(pwd)/.go/go-build:/.cache/go-build:delegated" \
		-e CGO_ENABLED=0 \
		-w /go/src/velero-plugin-for-csi \
		$(BUILD_IMAGE) \
		/bin/sh $(CMD)

build-dirs:
	@mkdir -p _output/bin/$(GOOS)/$(GOARCH)
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(GOOS)/$(GOARCH) .go/go-build

.PHONY: container
container: all build-dirs
	cp Dockerfile _output/bin/$(GOOS)/$(GOARCH)/Dockerfile
	docker build -t $(IMAGE) -f _output/bin/$(GOOS)/$(GOARCH)/Dockerfile _output/bin/$(GOOS)/$(GOARCH)

.PHONY: push
push: container
ifeq ($(TAG_LATEST), true)
	docker tag $(IMAGE_NAME):$(TAG) $(IMAGE_NAME):latest
	docker push $(IMAGE_NAME):latest
endif
	docker push $(IMAGE)

.PHONY: all-ci
all-ci: $(addprefix ci-, $(BIN))

.PHONY: modules
modules:
	go mod tidy

.PHONY: verify-modules
verify-modules: modules
	@if !(git diff --quiet HEAD -- go.sum go.mod); then \
		echo "go module files are out of date, please commit the changes to go.mod and go.sum"; exit 1; \
	fi

ci-%:
	$(MAKE) --no-print-directory BIN=$* ci

.PHONY: test
test: build-dirs
	@$(MAKE) shell  CMD="-c 'go test -timeout 30s -v -cover ./...'"

.PHONY: ci
ci: verify-modules all test
	IMAGE=velero-plugin-for-csi:pr-verify $(MAKE) container

changelog:
	hack/release-tools/changelog.sh

clean:
	@echo "cleaning"
	rm -rf .go _output
