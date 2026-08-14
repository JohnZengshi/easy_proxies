# easy_proxies build/run/package helpers.
GO ?= go
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
TAGS ?= with_utls with_quic with_grpc with_wireguard with_gvisor with_clash_api
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

EXE := $(if $(filter windows,$(GOOS)),.exe,)
BIN := runtime/easy_proxies$(EXE)
PP_BIN := runtime/proxypool$(EXE)
PP_DIR ?= $(abspath ../proxypool)

DIST_DIR := dist/easy_proxies-$(VERSION)-$(GOOS)-$(GOARCH)
ifeq ($(GOOS),windows)
PACKAGE_ARCHIVE := $(DIST_DIR).zip
PACKAGE_CMD := powershell -NoProfile -Command "Compress-Archive -Path '$(DIST_DIR)' -DestinationPath '$(PACKAGE_ARCHIVE)' -Force"
else
PACKAGE_ARCHIVE := $(DIST_DIR).tar.gz
PACKAGE_CMD := tar -czf $(PACKAGE_ARCHIVE) -C dist $(notdir $(DIST_DIR))
endif

ifeq ($(GOOS),windows)
PWSH := powershell -NoProfile -ExecutionPolicy Bypass
RUN_SCRIPT := $(PWSH) -File runtime/proxy-chain.ps1
RUN_DEPS := build
RUN_LEGACY_DEPS := build proxypool
else ifeq ($(GOOS),darwin)
RUN_SCRIPT := ./runtime/proxy-chain-macos.sh
RUN_DEPS := build
RUN_LEGACY_DEPS := build
else
RUN_SCRIPT := ./runtime/proxy-chain.sh
RUN_DEPS := build proxypool
RUN_LEGACY_DEPS := build proxypool
endif

.PHONY: all build proxypool run run-legacy stop restart status test vet fmt package clean

ifeq ($(GOOS),darwin)
all: build
else
all: build proxypool
endif

build:
	@mkdir -p runtime
	$(GO) build -tags "$(TAGS)" -o $(BIN) ./cmd/easy_proxies

proxypool:
	@test -d "$(PP_DIR)" || { echo "ERROR: proxypool source not found at $(PP_DIR)"; exit 1; }
	@mkdir -p runtime
	cd "$(PP_DIR)" && $(GO) build -tags "with_quic with_utls" -o "$(abspath $(PP_BIN))" ./cmd/proxypool

run: $(RUN_DEPS)
	$(RUN_SCRIPT) start

run-legacy: $(RUN_LEGACY_DEPS)
ifeq ($(GOOS),windows)
	$(PWSH) -File runtime/proxy-chain.ps1 start -Legacy
else
	$(RUN_SCRIPT) start
endif

stop:
	$(RUN_SCRIPT) stop

restart: $(RUN_DEPS)
	$(RUN_SCRIPT) restart

status:
	$(RUN_SCRIPT) status

test:
ifeq ($(GOOS),darwin)
	bash runtime/tests/sync-vpncheap-subscription.sh
	bash runtime/tests/proxy-chain-macos.sh
endif
	$(GO) test -tags "$(TAGS)" ./...

vet:
	$(GO) vet -tags "$(TAGS)" ./...

fmt:
	gofmt -w .

package: all
	@mkdir -p $(DIST_DIR)/runtime
ifeq ($(GOOS),darwin)
	cp $(BIN) $(DIST_DIR)/runtime/
	cp runtime/config.yaml runtime/proxy-chain-macos.sh runtime/sync-vpncheap-subscription.sh $(DIST_DIR)/runtime/
else
	cp $(BIN) $(PP_BIN) $(DIST_DIR)/runtime/
	cp runtime/config.yaml runtime/proxypool-config.yaml runtime/proxy-chain.sh runtime/proxy-chain.ps1 runtime/sync-vpncheap-subscription.ps1 $(DIST_DIR)/runtime/
endif
	cp README.md README_ZH.md $(DIST_DIR)/
	$(PACKAGE_CMD)
	@echo "package: $(PACKAGE_ARCHIVE)"

clean:
	rm -f $(BIN) $(PP_BIN)
	rm -rf dist
	$(GO) clean
