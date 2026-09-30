SED = $(shell which gsed 2>/dev/null || echo sed)
# Must stay in step with the optimism-package rev in
# devnet/kurtosis-devnet/optimism-package-trampoline/kurtosis.yml: op-deployer and the
# package's contract deployment have to agree, or DeploySuperchain reverts with
# "unrecognized 4 byte signature".
OP_TAG ?= op-node/v1.19.7
# L1 clients, kept identical to the cosmos-ethereum-ibc-lcp e2e devnet.
GETH_IMAGE ?= ethereum/client-go:v1.17.6
LODESTAR_IMAGE ?= ghcr.io/yoshidan/lodestar:v1.49.0-pr10022-9e69ac91

.PHONY: chain
chain:
	git clone --depth 1 -b $(OP_TAG) https://github.com/ethereum-optimism/optimism ./chain
	# Initialize git submodules (forge-std, etc.)
	cd chain && git submodule update --init --recursive --depth 1
	# devnet L1ChainConfig
	cp op-service/eth/config.go ./chain/op-service/eth/config.go
	# Fix Dockerfile (dhi.io typo)
	cp ops/docker/op-stack-go/Dockerfile ./chain/ops/docker/op-stack-go/Dockerfile
	# Generate the gitignored superchain-configs.zip embed from the superchain-registry
	# submodule (needs yq/jq/zip). optimism does not commit the zip, only its .sha256,
	# so a fresh clone must regenerate it or `go run cmd/main.go` fails on the //go:embed.
	cd chain && bash op-core/superchain/sync-superchain.sh
	# Sync dependencies
	cd devnet && go mod tidy

.PHONY: devnet-up
devnet-up:
	cd devnet/kurtosis-devnet && GETH_IMAGE=$(GETH_IMAGE) LODESTAR_IMAGE=$(LODESTAR_IMAGE) just simple-devnet

.PHONY: devnet-down
devnet-down:
	@ENCLAVE=$$(kurtosis enclave ls | awk 'NR==2 {print $$1}'); kurtosis enclave rm -f $$ENCLAVE
	kurtosis engine stop
