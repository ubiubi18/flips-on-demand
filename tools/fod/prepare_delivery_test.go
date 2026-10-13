package main

import (
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAssembleDeliveryAcceptsAProvenOrder ties the CLI's assembly to a proof the
// prover actually produced: the calldata must be that artefact's own argument,
// byte for byte.
func TestAssembleDeliveryAcceptsAProvenOrder(t *testing.T) {
	path := filepath.Join("..", "..", "proofs", "delivery", "dist", "delivery-v2.json")
	artefact, err := readProverArtefact(path)
	if err != nil {
		t.Fatalf("read the committed delivery artefact: %v", err)
	}
	request := &actionRequest{Network: "idena-local", Contract: artefact.Order.Contract}
	arguments := &fulfillArguments{
		OfferID:       artefact.Order.OfferID,
		OrderNonce:    artefact.Order.OrderNonce,
		Seller:        artefact.Order.Seller,
		Buyer:         artefact.Order.Buyer,
		DeliveryKey:   artefact.Order.BuyerKey,
		FundingHeight: artefact.Order.FundedAt,
		FundingRoot:   artefact.Order.FundingRoot,
		Epoch:         205,
		Author:        artefact.Order.Seller,
	}

	prepared, err := AssembleDelivery(request, arguments, artefact)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if prepared.Calldata != artefact.FulfillArgument {
		t.Fatal("prepared calldata differs from the prover's argument")
	}
	if prepared.Format != preparedDeliveryFormat || prepared.Operation != operationFulfill {
		t.Fatalf("unexpected artefact: %s / %s", prepared.Format, prepared.Operation)
	}
	if prepared.OfferID != artefact.Order.OfferID || prepared.OrderNonce != artefact.Order.OrderNonce {
		t.Fatal("prepared action names another order")
	}
	if prepared.Submitted {
		t.Fatal("the CLI claims to have submitted something it cannot submit")
	}

	// A journal naming another order must be refused rather than prepared.
	journal, err := hex.DecodeString(artefact.Journal)
	if err != nil {
		t.Fatalf("journal hex: %v", err)
	}
	binary.BigEndian.PutUint64(journal[24:32], 99)
	broken := *artefact
	broken.Journal = hex.EncodeToString(journal)
	if _, err := AssembleDelivery(request, arguments, &broken); err == nil {
		t.Fatal("a journal naming another order was assembled")
	}

	// So must an argument that is not the layout the escrow parses.
	short := *artefact
	short.FulfillArgument = artefact.FulfillArgument[:len(artefact.FulfillArgument)-2]
	if _, err := AssembleDelivery(request, arguments, &short); err == nil {
		t.Fatal("a truncated fulfill argument was assembled")
	}

	for _, mutate := range []func(*actionRequest, *fulfillArguments, *proverArtefact){
		func(r *actionRequest, a *fulfillArguments, p *proverArtefact) { r.Contract = strings.Repeat("ff", 20) },
		func(r *actionRequest, a *fulfillArguments, p *proverArtefact) { a.FundingHeight++ },
		func(r *actionRequest, a *fulfillArguments, p *proverArtefact) {
			a.FundingRoot = strings.Repeat("ff", 32)
		},
		func(r *actionRequest, a *fulfillArguments, p *proverArtefact) {
			a.DeliveryKey = strings.Repeat("04", 65)
		},
		func(r *actionRequest, a *fulfillArguments, p *proverArtefact) {
			p.FulfillArgument = p.FulfillArgument[:20] + "ff" + p.FulfillArgument[22:]
		},
		func(r *actionRequest, a *fulfillArguments, p *proverArtefact) { p.Statement = "fod:delivery:v1" },
	} {
		r, a, p := *request, *arguments, *artefact
		mutate(&r, &a, &p)
		if _, err := AssembleDelivery(&r, &a, &p); err == nil {
			t.Fatal("mismatched domain, terms or embedded calldata accepted")
		}
	}

	// And a development-mode receipt, which carries no Groth16 seal at all
	// (`InnerReceipt::Fake`): the escrow decodes a seal of its own from the
	// argument, so one that is not there has to stop at this boundary.
	noSeal := *artefact
	noSeal.Seal = ""
	noSeal.SealBytes = 0
	if _, err := AssembleDelivery(request, arguments, &noSeal); err == nil ||
		!strings.Contains(err.Error(), "seal") {
		t.Fatalf("a receipt with no Groth16 seal was assembled: %v", err)
	}
}

// TestRunDeliveryProverFailsWithoutABinary checks the failure path: no proof
// artefact is left behind when the prover cannot run.
func TestRunDeliveryProverFailsWithoutABinary(t *testing.T) {
	dir := t.TempDir()
	proof := filepath.Join(dir, "delivery.proof.json")
	if _, err := runDeliveryProver(filepath.Join(dir, "missing-prover"), proof, []byte("{}")); err == nil {
		t.Fatal("a missing prover binary was reported as a success")
	}
	if _, err := os.Stat(proof); !os.IsNotExist(err) {
		t.Fatalf("a proof artefact was left behind: %v", err)
	}
}
