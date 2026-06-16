SED = $(shell which gsed 2>/dev/null || echo sed)
OP_TAG ?= op-node/v1.19.0
LODESTAR_VERSION ?= v1.41.1
GETH_VERSION ?= v1.17.3

.PHONY: chain
chain:
	git clone --depth 1 -b $(OP_TAG) https://github.com/ethereum-optimism/optimism ./chain
	# Initialize git submodules (forge-std, etc.)
	cd chain && git submodule update --init --recursive --depth 1
	# devnet L1ChainConfig
	cp op-service/eth/config.go ./chain/op-service/eth/config.go
	# Fix Dockerfile (dhi.io typo)
	cp ops/docker/op-stack-go/Dockerfile ./chain/ops/docker/op-stack-go/Dockerfile
	# Sync dependencies
	cd devnet && go mod tidy

.PHONY: devnet-up
devnet-up:
	cd devnet/kurtosis-devnet && GETH_VERSION=$(GETH_VERSION) LODESTAR_VERSION=$(LODESTAR_VERSION) just simple-devnet

.PHONY: devnet-down
devnet-down:
	@ENCLAVE=$$(kurtosis enclave ls | awk 'NR==2 {print $$1}'); kurtosis enclave rm -f $$ENCLAVE
	kurtosis engine stop
