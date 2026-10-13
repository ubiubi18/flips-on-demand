package harness

// Cross-check for the money-out half of the file-based workflow: the calls the
// CLI prepares from a pool `refund` and a `withdraw` request must be the calls
// the escrow accepts.
//
// The two do not share an argument encoding, which is the point of checking
// both: `refund` reads the order id as decimal text, and `withdraw` takes no
// argument at all. A CLI that guessed either one would fail here rather than on
// a live chain.
//
// The artefacts are produced by tools/fod; see docs/PROGRESS.md. The tests skip
// when they have not been generated.

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

type preparedClaimArtifact struct {
	Format    string `json:"format"`
	Operation string `json:"operation"`
	Network   string `json:"network"`
	Contract  string `json:"contract"`
	OfferID   uint64 `json:"offerId"`
	Amount    string `json:"amount"`
	Calldata  string `json:"calldata"`
	Submitted bool   `json:"submitted"`
}

func loadPreparedClaim(t *testing.T, name, format, operation string) *preparedClaimArtifact {
	t.Helper()
	path := fmt.Sprintf("../../output/cli/%s", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("prepared claim not generated (%v); run the CLI first", err)
	}
	var claim preparedClaimArtifact
	if err := json.Unmarshal(raw, &claim); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if claim.Format != format || claim.Operation != operation {
		t.Fatalf("unexpected artefact: %s / %s", claim.Format, claim.Operation)
	}
	if claim.Submitted {
		t.Fatal("the CLI claims to have submitted something it cannot submit")
	}
	if claim.Amount != "0" {
		t.Fatalf("a claim sends no value, the artefact says %q", claim.Amount)
	}
	return &claim
}

func TestEscrowAcceptsCliPreparedPoolRefund(t *testing.T) {
	claim := loadPreparedClaim(t, "prepared-pool-refund.json", "fod.prepared-refund-order", "refund")
	calldata := mustHex(t, claim.Calldata)
	// The escrow parses this argument as decimal text, not as eight bytes.
	if string(calldata) != fmt.Sprintf("%d", claim.OfferID) {
		t.Fatalf("the refund calldata is %q, expected order %d in decimal", calldata, claim.OfferID)
	}

	code, err := os.ReadFile(escrowWasmPath)
	if err != nil {
		t.Fatalf("read escrow module: %v", err)
	}
	env := NewMinimalEnv()
	env.SetContractAddress(addressFromHex(t, claim.Contract))
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)

	commitment := make([]byte, 32)
	for i := range commitment {
		commitment[i] = byte(0x50 + i)
	}
	env.SetBlockNumber(1)
	reserve := new(big.Int).Lsh(testPriceValue, 1)
	seed := join(commitment, u128ArgValue(reserve), u64Arg(2), u64Arg(1))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle", seed); !strings.Contains(out, "true") {
		t.Fatalf("seed the pool bundle failed: %s", out)
	}
	env.CreditContractBalance(testPriceValue)
	buyer := CallerAddress()
	order := join(u64Arg(claim.OfferID), commitment, buyer[:], make([]byte, 65), u128ArgValue(testPriceValue), u64Arg(1), u64Arg(0))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolOrder", order); !strings.Contains(out, "true") {
		t.Fatalf("seed the pool order failed: %s", out)
	}

	// Too early: the escrow refuses before the deadline plus grace, by failing
	// the call (`REVIEW.md` A04).
	escrowReject(t, env, api, code, "refund", calldata)
	env.SetBlockNumber(1 + poolDeadlineBlocks + 1)

	gas, out := escrowCallBytes(t, env, api, code, "refund", calldata)
	if !strings.Contains(out, "true") {
		t.Fatalf("the escrow refused the CLI's own refund calldata: %s", out)
	}
	t.Logf("CLI pool refund calldata accepted by the escrow, gas=%d", gas)

	_, accounting := escrowCall(t, env, api, code, "accounting", "0")
	if !strings.Contains(accounting, `"callerCredit":0`) || !strings.Contains(accounting, `"pendingTotal":0`) {
		t.Fatalf("the refund left escrow behind: %s", accounting)
	}
	// The refund pays the buyer directly in one transaction; no second
	// withdrawal step is needed.
	if got := env.BalanceOf(buyer); got.Cmp(testPriceValue) != 0 {
		t.Fatalf("the refund did not pay the buyer: %s", got)
	}
	t.Logf("accounting after the CLI's pool refund: %s", accounting)
}

func TestEscrowAcceptsCliPreparedWithdrawal(t *testing.T) {
	claim := loadPreparedClaim(t, "prepared-withdrawal.json", "fod.prepared-withdrawal", "withdraw")
	if claim.Calldata != "" {
		t.Fatalf("a withdrawal takes no argument, the artefact carries %q", claim.Calldata)
	}
	calldata := mustHex(t, claim.Calldata)

	code, err := os.ReadFile(escrowWasmPath)
	if err != nil {
		t.Fatalf("read escrow module: %v", err)
	}
	env := NewMinimalEnv()
	env.SetContractAddress(addressFromHex(t, claim.Contract))
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)
	creditCaller(t, env, api, code)

	gas, out := escrowCallBytes(t, env, api, code, "withdraw", calldata)
	if !strings.Contains(out, "true") {
		t.Fatalf("the escrow refused the CLI's own withdrawal calldata: %s", out)
	}
	t.Logf("CLI withdrawal calldata accepted by the escrow, gas=%d", gas)

	if got := env.BalanceOf(CallerAddress()); got.Cmp(testFeeValue) != 0 {
		t.Fatalf("the caller was paid %s, expected the whole credit %s", got, testFeeValue)
	}
	if got := env.ContractBalance(); got.Cmp(testPriceValue) != 0 {
		t.Fatalf("the contract should hold only the pool's reserve after paying out: %s", got)
	}
	// The credit is spent: the same call again finds nothing to pay, and refuses
	// by failing the call.
	escrowReject(t, env, api, code, "withdraw", calldata)
}
