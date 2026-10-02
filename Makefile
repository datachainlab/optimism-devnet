SED = $(shell which gsed 2>/dev/null || echo sed)
# Baseline tag; one tag pins the whole stack, since op-node/v1.19.7 is the same commit
# as op-reth/v2.4.4, op-batcher/v1.17.0 and kona-node/v1.7.0. Callers may override it,
# but two things have to keep agreeing with whatever they pick:
#   - the op-geth replace in devnet/go.mod. A mismatch fails the build on fields the
#     older op-geth does not know (e.g. LagoonTime).
#   - the optimism-package rev in
#     devnet/kurtosis-devnet/optimism-package-trampoline/kurtosis.yml. op-deployer and
#     the package's contract deployment have to agree, or DeploySuperchain reverts with
#     "unrecognized 4 byte signature".
# op-node/v1.19.8 (= op-reth/v2.5.0) is known to work: same op-geth, same package rev.
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
	# Wolfi moved ca-certificates onto openssl-4.0-libcrypto, which owns /etc/ssl/*.cnf,
	# so pulling the openssl 3.x package alongside it aborts with "trying to overwrite".
	# op-reth links neither libssl nor libcrypto, so matching the 4.0 line is enough.
	# Patched by substitution rather than by copying the file, since OP_TAG is overridable.
	$(SED) -i 's/ca-certificates openssl libstdc++/ca-certificates openssl-4.0 libstdc++/' \
		./chain/rust/op-reth/DockerfileOp
	@grep -q 'ca-certificates openssl-4.0 libstdc++' ./chain/rust/op-reth/DockerfileOp \
		|| { echo "op-reth DockerfileOp: apk line not patched, upstream must have changed it"; exit 1; }
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
