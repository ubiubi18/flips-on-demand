package harness

// Minimal HostEnv for the contract ABI spike.
//
// Only the callbacks the spike module actually uses are implemented. Everything
// else is a stub that returns zero values. This is deliberate: the spike
// measures runtime acceptance, gas metering, arithmetic cost, and the escrow
// accounting rules, not the full node.
//
// Balances are modelled per address, because the node debits the contract's own
// balance and can credit whichever address it is given (`idena-go`
// `vm/wasm/wasm_env.go`, SubBalance/AddBalance). A sub-env shares this state the
// way the node's sub-envs share one appState; the node saves a call's writes
// only when the call succeeded, and a test that expects a failure models that
// by calling Snapshot's restore function.

import (
	"crypto/sha256"
	"errors"
	"math/big"
	"sort"

	gethtcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/idena-network/idena-wasm-binding/lib"
)

// Host costs the node charges for the two cryptographic callbacks, in contract
// gas, and the multiplier the node uses to turn them into wasm gas
// (idena-go vm/costs/costs.go: ComputeHashGas, EcrecoverGas, WasmGasMultiplier).
const (
	costsComputeHashGas = 100
	costsEcrecoverGas   = 300
	// costsReadBlockGas is `vm/costs.ReadBlockGas`, what the node charges for a
	// header, an amount or a balance read before the wasm multiplier.
	costsReadBlockGas         = 5
	costsReadStatePerByteGas  = 10
	costsWriteStatePerByteGas = 20
	costsRemoveStateGas       = 5
	wasmGasMultiplier         = 100
)

func chargeHostGas(m *lib.GasMeter, contractGas uint64) {
	if m != nil {
		m.ConsumeGas(contractGas * wasmGasMultiplier)
	}
}

// The harness keeps the contract address and the caller address apart, as the
// node does: Balance() reads the contract's own balance and add_balance credits
// the receiver it is handed.
var (
	contractAddr = lib.Address{1}
	callerAddr   = lib.Address{2}
)

// sharedState is what nested calls see. The node keeps one appState for a whole
// call tree, so storage and balances are shared rather than copied.
type sharedState struct {
	storage  map[string][]byte
	code     []byte
	balances map[lib.Address]*big.Int
}

type MinimalEnv struct {
	state       *sharedState
	contract    lib.Address
	caller      lib.Address
	feePerGas   *big.Int
	commits     int
	subEnvs     int
	depth       int
	blockNumber uint64
	epochNumber uint16
	payAmount   *big.Int
	// blockHeaders is the chain data a node answers `block_header(height)` from.
	// It is not call state, so snapshots and rollback do not touch it.
	blockHeaders map[uint64][]byte
}

func NewMinimalEnv() *MinimalEnv {
	return &MinimalEnv{
		state: &sharedState{
			storage:  map[string][]byte{},
			balances: map[lib.Address]*big.Int{contractAddr: big.NewInt(0)},
		},
		contract:     contractAddr,
		caller:       callerAddr,
		feePerGas:    big.NewInt(1),
		blockNumber:  1,
		payAmount:    big.NewInt(0),
		blockHeaders: map[uint64][]byte{},
	}
}

// SetBlockNumber drives deadline logic in tests.
func (e *MinimalEnv) SetBlockNumber(n uint64) { e.blockNumber = n }

func (e *MinimalEnv) SetEpoch(n uint16) { e.epochNumber = n }

// SetCaller switches the address the contract sees as the sender, which is what
// a buyer-bound proof has to be tied to.
func (e *MinimalEnv) SetCaller(addr lib.Address) { e.caller = addr }

// SetContractAddress switches the address the contract sees as its own, which
// the delivery statement names.
func (e *MinimalEnv) SetContractAddress(addr lib.Address) { e.contract = addr }

// ContractAddressValue is the address this environment presents to the contract.
func (e *MinimalEnv) ContractAddressValue() lib.Address { return e.contract }

// SetPayAmount sets the value a call carries, which the pool's buy compares with the
// offer price.
func (e *MinimalEnv) SetPayAmount(v *big.Int) {
	e.payAmount = new(big.Int).Set(v)
}

// ResetPayAmount models the end of a call: the amount a transaction carries
// belongs to that call, so the next call starts with none.
func (e *MinimalEnv) ResetPayAmount() {
	e.payAmount = new(big.Int)
}

// CreditContractBalance models the value transfer a payable call performs: the
// host moves the carried amount into the contract's balance. Without this the
// liquidity invariant would be checked against a balance that never moved.
func (e *MinimalEnv) CreditContractBalance(v *big.Int) {
	e.setBalance(e.contract, new(big.Int).Add(e.balanceOf(e.contract), v))
}

// ContractBalance exposes the modelled contract balance for assertions.
func (e *MinimalEnv) ContractBalance() *big.Int {
	return e.balanceOf(e.contract)
}

// SetContractBalance lets a test model a contract that cannot cover its
// liabilities, which is how the refused-payout path is exercised.
func (e *MinimalEnv) SetContractBalance(v *big.Int) {
	e.setBalance(e.contract, new(big.Int).Set(v))
}

// CallerAddress is the address a contract call sees as its sender.
func CallerAddress() lib.Address { return callerAddr }

// BalanceOf reads the modelled balance of any address, so a test can check who
// a payout credited.
func (e *MinimalEnv) BalanceOf(addr lib.Address) *big.Int { return e.balanceOf(addr) }

// SubEnvCount reports how many sub-environments were created, which is how a
// test detects that a nested call, such as a promise callback, was made.
func (e *MinimalEnv) SubEnvCount() int { return e.subEnvs }

// Snapshot copies the modelled state and returns a restore function. The node
// keeps a failed call's writes out of state (`idena-go` `vm/wasm/vm.go`:
// `if err == nil && vm.commitToState`), so a test that expects a failure calls
// the returned function to model that rule.
func (e *MinimalEnv) Snapshot() func() {
	storage := copyStorage(e.state.storage)
	balances := copyBalances(e.state.balances)
	code := append([]byte(nil), e.state.code...)
	return func() {
		e.state.storage = storage
		e.state.balances = balances
		e.state.code = code
	}
}

// StateDigest digests everything the node keeps when a call succeeds: contract
// storage and every modelled balance. A refused call must leave it unchanged,
// which is how a write that happens *before* a failure is caught instead of
// being hidden by the rollback the node would perform.
func (e *MinimalEnv) StateDigest() string {
	hasher := sha256.New()
	storageKeys := make([]string, 0, len(e.state.storage))
	for key := range e.state.storage {
		storageKeys = append(storageKeys, key)
	}
	sort.Strings(storageKeys)
	for _, raw := range storageKeys {
		hasher.Write([]byte(raw))
		hasher.Write([]byte{0})
		hasher.Write(e.state.storage[raw])
		hasher.Write([]byte{1})
	}
	addresses := make([]string, 0, len(e.state.balances))
	for address := range e.state.balances {
		addresses = append(addresses, string(address[:]))
	}
	sort.Strings(addresses)
	for _, raw := range addresses {
		var address lib.Address
		copy(address[:], raw)
		hasher.Write([]byte(raw))
		hasher.Write([]byte{0})
		hasher.Write(e.state.balances[address].Bytes())
		hasher.Write([]byte{1})
	}
	return string(hasher.Sum(nil))
}

// Restart models a node restart: the modelled state is copied into a fresh
// environment, so nothing can carry over except what a node would reload.
func (e *MinimalEnv) Restart() *MinimalEnv {
	fresh := NewMinimalEnv()
	fresh.state.storage = copyStorage(e.state.storage)
	fresh.state.balances = copyBalances(e.state.balances)
	fresh.state.code = append([]byte(nil), e.state.code...)
	// The contract's own address and the node's fee are configuration a restart
	// reloads; the caller belongs to the next call and starts fresh. Without this
	// a test that deployed the module at its own address would look for the
	// contract's balance at the default address after the restart.
	fresh.contract = e.contract
	fresh.feePerGas = new(big.Int).Set(e.feePerGas)
	fresh.blockNumber = e.blockNumber
	fresh.payAmount = new(big.Int).Set(e.payAmount)
	for height, header := range e.blockHeaders {
		fresh.blockHeaders[height] = append([]byte(nil), header...)
	}
	return fresh
}

func copyStorage(src map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(src))
	for k, v := range src {
		out[k] = append([]byte(nil), v...)
	}
	return out
}

func copyBalances(src map[lib.Address]*big.Int) map[lib.Address]*big.Int {
	out := make(map[lib.Address]*big.Int, len(src))
	for k, v := range src {
		out[k] = new(big.Int).Set(v)
	}
	return out
}

func (e *MinimalEnv) balanceOf(addr lib.Address) *big.Int {
	if b, ok := e.state.balances[addr]; ok {
		return new(big.Int).Set(b)
	}
	return big.NewInt(0)
}

func (e *MinimalEnv) setBalance(addr lib.Address, v *big.Int) {
	e.state.balances[addr] = new(big.Int).Set(v)
}

func (e *MinimalEnv) SetStorage(m *lib.GasMeter, key []byte, value []byte) {
	e.state.storage[string(key)] = append([]byte(nil), value...)
	chargeHostGas(m, uint64(costsWriteStatePerByteGas*(len(key)+len(value))))
}

func (e *MinimalEnv) GetStorage(m *lib.GasMeter, key []byte) []byte {
	value := e.state.storage[string(key)]
	chargeHostGas(m, uint64(costsReadStatePerByteGas*len(value)))
	return append([]byte(nil), value...)
}

func (e *MinimalEnv) RemoveStorage(m *lib.GasMeter, key []byte) {
	delete(e.state.storage, string(key))
	chargeHostGas(m, costsRemoveStateGas)
}

func (e *MinimalEnv) BlockNumber(m *lib.GasMeter) uint64 { return e.blockNumber }

func (e *MinimalEnv) BlockTimestamp(m *lib.GasMeter) int64 { return 0 }

func (e *MinimalEnv) MinFeePerGas(m *lib.GasMeter) *big.Int { return e.feePerGas }

func (e *MinimalEnv) Balance(m *lib.GasMeter) *big.Int { return e.balanceOf(e.contract) }

func (e *MinimalEnv) BlockSeed(m *lib.GasMeter) []byte { return nil }

func (e *MinimalEnv) NetworkSize(m *lib.GasMeter) uint64 { return 0 }

func (e *MinimalEnv) Identity(m *lib.GasMeter, address lib.Address) []byte { return nil }

func (e *MinimalEnv) CreateSubEnv(contract lib.Address, method string, payAmount *big.Int, isDeploy bool) (lib.HostEnv, error) {
	e.subEnvs++
	return &MinimalEnv{
		state:        e.state,
		contract:     contract,
		caller:       e.caller,
		feePerGas:    e.feePerGas,
		depth:        e.depth + 1,
		blockNumber:  e.blockNumber,
		epochNumber:  e.epochNumber,
		payAmount:    e.payAmount,
		blockHeaders: e.blockHeaders,
	}, nil
}

// GetCode returns the module a nested call executes. Without it a nested call
// would only report "code is empty".
func (e *MinimalEnv) GetCode(addr lib.Address) []byte { return e.state.code }

func (e *MinimalEnv) Commit() { e.commits++ }

func (e *MinimalEnv) Caller(m *lib.GasMeter) lib.Address { return e.caller }

func (e *MinimalEnv) OriginalCaller(m *lib.GasMeter) lib.Address { return e.caller }

// SubBalance debits the contract's own balance and refuses to overdraw, which
// is what makes an unbacked withdrawal abort the whole call.
func (e *MinimalEnv) SubBalance(m *lib.GasMeter, amount *big.Int) error {
	if amount.Sign() < 0 {
		return errors.New("value must be non-negative")
	}
	balance := e.balanceOf(e.contract)
	if balance.Cmp(amount) < 0 {
		return errors.New("insufficient funds")
	}
	e.setBalance(e.contract, new(big.Int).Sub(balance, amount))
	return nil
}

// AddBalance credits the address the caller named.
func (e *MinimalEnv) AddBalance(m *lib.GasMeter, address lib.Address, amount *big.Int) {
	e.setBalance(address, new(big.Int).Add(e.balanceOf(address), amount))
}

func (e *MinimalEnv) ContractAddress(m *lib.GasMeter) lib.Address { return e.contract }

func (e *MinimalEnv) ContractAddr(m *lib.GasMeter, code []byte, args []byte, nonce []byte) lib.Address {
	return e.contract
}

func (e *MinimalEnv) Deploy(code []byte) { e.state.code = append([]byte(nil), code...) }

func (e *MinimalEnv) ContractAddrByHash(m *lib.GasMeter, hash []byte, args []byte, nonce []byte) lib.Address {
	return lib.Address{}
}

func (e *MinimalEnv) OwnCode(m *lib.GasMeter) []byte { return nil }

func (e *MinimalEnv) CodeHash(m *lib.GasMeter) []byte { return nil }

func (e *MinimalEnv) Event(m *lib.GasMeter, name string, args ...[]byte) {}

func (e *MinimalEnv) ReadContractData(m *lib.GasMeter, address lib.Address, key []byte) []byte {
	return nil
}

func (e *MinimalEnv) Epoch(m *lib.GasMeter) uint16 { return e.epochNumber }

func (e *MinimalEnv) ContractCodeHash(addr lib.Address) *[]byte { return nil }

func (e *MinimalEnv) PayAmount(m *lib.GasMeter) *big.Int { return e.payAmount }

func (e *MinimalEnv) IsDebug() bool { return false }

// SetBlockHeader binds the header a node would answer for one height.
func (e *MinimalEnv) SetBlockHeader(height uint64, header []byte) {
	e.blockHeaders[height] = append([]byte(nil), header...)
}

// BlockHeader mirrors `idena-go` `vm/wasm/wasm_env.go` BlockHeader: the header
// at that height, protobuf-encoded, charged at ReadBlockGas.
func (e *MinimalEnv) BlockHeader(m *lib.GasMeter, height uint64) []byte {
	chargeHostGas(m, costsReadBlockGas)
	return append([]byte(nil), e.blockHeaders[height]...)
}

// Keccak256 mirrors the node callback: legacy Keccak-256 over the given bytes,
// charged at costs.ComputeHashGas.
func (e *MinimalEnv) Keccak256(m *lib.GasMeter, data []byte) []byte {
	chargeHostGas(m, costsComputeHashGas)
	return gethtcrypto.Keccak256(data)
}

func (e *MinimalEnv) GlobalState(m *lib.GasMeter) []byte { return nil }

func (e *MinimalEnv) Burn(m *lib.GasMeter, amount *big.Int) error { return nil }

// Ecrecover mirrors the node callback: the 65-byte uncompressed public key that
// signed the 32-byte digest, or nothing when recovery fails. The node returns
// whatever crypto.Ecrecover produced, errors included as nil
// (idena-go vm/wasm/wasm_env.go), and the runtime then reports a zero region.
func (e *MinimalEnv) Ecrecover(m *lib.GasMeter, data []byte, signature []byte) []byte {
	chargeHostGas(m, costsEcrecoverGas)
	pub, err := gethtcrypto.Ecrecover(data, signature)
	if err != nil {
		return nil
	}
	return pub
}
