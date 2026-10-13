module github.com/ubiubi18/flips-on-demand/spikes/harness

// The harness runs against a read-only checkout of the contract runtime
// binding, which is not vendored. Local development: copy this file to
// go.local.mod, append a replace for your checkout, and build with
// GOFLAGS=-modfile=go.local.mod. See README.md.

go 1.26.5

require (
	github.com/ethereum/go-ethereum v1.16.1
	github.com/idena-network/idena-wasm-binding v0.0.0-20260710092321-f7c71ce32d86
)

require (
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
