# optimism-devnet

Kurtosis-based devnet for Optimism development.

## Requirements

- **kurtosis**: 1.15.2

## Usage

### Setup

Clone the Optimism repository and initialize dependencies:

```bash
make chain
```

To use a specific version:

```bash
OP_TAG=op-node/v1.19.5 make chain
```

### Start Devnet

```bash
make devnet-up
```

To use specific client versions:

```bash
GETH_VERSION=v1.17.3 LODESTAR_VERSION=v1.46.0 make devnet-up
```

### Stop Devnet

```bash
make devnet-down
```

## Configuration

### Hardfork Timestamps

To modify hardfork timestamps, edit the following files:

- **`devnet/kurtosis-devnet/simple.yaml`** - L1/L2 network parameters (e.g., `fulu_fork_epoch`, `fjord_time_offset`, `granite_time_offset`, etc.)
- **`op-service/eth/config.go`** - L1 chain config for devnet (e.g., `PragueTime`, `OsakaTime`, etc.)
