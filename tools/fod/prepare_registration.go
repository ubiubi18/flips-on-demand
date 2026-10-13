package main

// The author's registration step in `SPEC.md` 4.3: a sealed registration
// receipt is what 5.5 asks a bundle to prove, and this turns that artefact into
// the `registerBundle` call the escrow takes (journal || seal).
//
// The receipt is the author's own; nothing here can substitute for it. This
// command checks the artefact's shape and the two labels the escrow pins,
// records the claim the receipt proves, and refuses an artefact whose journal
// and seal do not belong together. Who may send the call is enforced by the
// contract: the caller has to be the author the journal names.

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	operationRegisterBundle        = "registerBundle"
	preparedPoolRegistrationFormat = "fod.prepared-register-bundle"
	registrationStatement          = "fod:registration:v3"

	// The escrow parses the registration argument in this layout, so the sizes
	// are pinned here as well as in the contract and the prover.
	registrationSealBytes = 256

	// Field offsets inside the 326-byte version 3 journal
	// (`proofs/session-core/src/statement.rs`, `SOURCES.md` F-97).
	registrationContractOffset        = 4
	registrationAuthorOffset          = 24
	registrationEpochOffset           = 44
	registrationCoveredOffset         = 46
	registrationInventoryHeightOffset = 50
	registrationInventoryRootOffset   = 58
	registrationVerdictHeightOffset   = 90
	registrationVerdictRootOffset     = 98
	registrationFlagsOffset           = 130
	registrationManifestOffset        = 134
	registrationCommitmentOffset      = 166
	registrationInventoryOffset       = 198
	registrationPolicyOffset          = 230
	registrationConsensusOffset       = 262
	registrationNetworkOffset         = 294
)

// registrationArtefact is what `proofs/registration/host` writes: the statement,
// the journal, and the Groth16 seal, together with the digests the host verified
// against the guest it ran.
type registrationArtefact struct {
	Statement    string  `json:"statement"`
	Journal      string  `json:"journal"`
	Seal         string  `json:"seal"`
	SealBytes    int     `json:"sealBytes"`
	ClaimDigest  string  `json:"claim_digest"`
	ImageID      string  `json:"image_id"`
	TotalCycles  uint64  `json:"totalCycles"`
	Segments     int     `json:"segments"`
	ProveSeconds float64 `json:"proveSeconds"`
}

// PreparedRegistration is what the author's wallet submits: the claim the
// receipt proves, plus the exact argument bytes for `registerBundle`.
type PreparedRegistration struct {
	Format             string `json:"format"`
	Version            int    `json:"version"`
	Operation          string `json:"operation"`
	Network            string `json:"network"`
	Contract           string `json:"contract"`
	Author             string `json:"author"`
	Epoch              uint16 `json:"epoch"`
	CoveredFlips       uint32 `json:"coveredFlips"`
	ManifestCommitment string `json:"manifestCommitment"`
	KeyCommitment      string `json:"keyCommitment"`
	// InventoryCommitment is what the pool record republishes, so a buyer can
	// check a candidate CID set against chain state.
	InventoryCommitment string `json:"inventoryCommitment"`
	PolicyDigest        string `json:"policyDigest"`
	ConsensusDigest     string `json:"consensusDigest"`
	NetworkDigest       string `json:"networkDigest"`
	ImageID             string `json:"imageId,omitempty"`
	ClaimDigest         string `json:"claimDigest,omitempty"`
	Calldata            string `json:"calldata"`
	Amount              string `json:"amount"`
	Proof               string `json:"proof"`
	PreparedAt          string `json:"preparedAt"`
	Submitted           bool   `json:"submitted"`
}

// readRegistrationArtefact reads the prover's artefact.
func readRegistrationArtefact(path string) (*registrationArtefact, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var artefact registrationArtefact
	if err := json.Unmarshal(raw, &artefact); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &artefact, nil
}

// AssemblePoolRegistration turns a registration receipt into the ownerless
// pool's `registerBundle` call. expectedContract, when given, is the registry
// the caller believes it is registering in; a receipt for another registry is
// refused, not sent.
func AssemblePoolRegistration(network, expectedContract, proofPath string, artefact *registrationArtefact) (*PreparedRegistration, error) {
	return assembleRegistration(network, expectedContract, proofPath, artefact, operationRegisterBundle, preparedPoolRegistrationFormat, registrationImageIDHex)
}

func assembleRegistration(network, expectedContract, proofPath string, artefact *registrationArtefact, operation, format, expectedImage string) (*PreparedRegistration, error) {
	if artefact.Statement != registrationStatement {
		return nil, fmt.Errorf("the artefact is not a registration proof: %q", artefact.Statement)
	}
	journal, err := hex.DecodeString(strings.TrimPrefix(artefact.Journal, "0x"))
	if err != nil {
		return nil, fmt.Errorf("artefact journal: %w", err)
	}
	seal, err := hex.DecodeString(strings.TrimPrefix(artefact.Seal, "0x"))
	if err != nil {
		return nil, fmt.Errorf("artefact seal: %w", err)
	}
	if len(journal) != registrationJournalBytes {
		return nil, fmt.Errorf("artefact journal is %d bytes, expected %d", len(journal), registrationJournalBytes)
	}
	if len(seal) != registrationSealBytes {
		return nil, fmt.Errorf("artefact seal is %d bytes, expected %d", len(seal), registrationSealBytes)
	}
	if artefact.SealBytes != 0 && artefact.SealBytes != len(seal) {
		return nil, fmt.Errorf("the artefact says its seal is %d bytes, it is %d", artefact.SealBytes, len(seal))
	}
	if version := binary.BigEndian.Uint32(journal[:4]); version != registrationStatementVersion {
		return nil, fmt.Errorf("the receipt commits statement version %d", version)
	}

	contract := hex.EncodeToString(journal[registrationContractOffset : registrationContractOffset+addressBytes])
	author := hex.EncodeToString(journal[registrationAuthorOffset : registrationAuthorOffset+addressBytes])
	epoch := binary.BigEndian.Uint16(journal[registrationEpochOffset : registrationEpochOffset+2])
	covered := binary.BigEndian.Uint32(journal[registrationCoveredOffset : registrationCoveredOffset+4])
	if covered == 0 {
		return nil, errors.New("the receipt covers no flips")
	}
	if expectedContract != "" && !strings.EqualFold(strings.TrimPrefix(expectedContract, "0x"), contract) {
		return nil, fmt.Errorf("the receipt names contract %s, not %s", contract, expectedContract)
	}

	// The escrow pins both labels. A receipt binding another policy or another
	// consensus revision can never register, so it is refused here rather than
	// turned into a call the node would reject.
	policy := hex.EncodeToString(journal[registrationPolicyOffset : registrationPolicyOffset+32])
	if want := labelDigest(EligibilityPolicyV1); policy != want {
		return nil, fmt.Errorf("the receipt binds eligibility policy %s, the release pins %s", policy, want)
	}
	consensus := hex.EncodeToString(journal[registrationConsensusOffset : registrationConsensusOffset+32])
	if want := labelDigest(ConsensusVersionPinned); consensus != want {
		return nil, fmt.Errorf("the receipt binds consensus version %s, the release pins %s", consensus, want)
	}
	// The chain binding: the statement names the network its evidence came from,
	// and the escrow pins the same digest.
	networkDigest := hex.EncodeToString(journal[registrationNetworkOffset : registrationNetworkOffset+32])
	if want := labelDigest(NetworkNamePinned); networkDigest != want {
		return nil, fmt.Errorf("the receipt binds network %s, and this release pins %s", networkDigest, want)
	}
	if artefact.ImageID != "" && !isHex(artefact.ImageID, 32) {
		return nil, errors.New("the artefact's image id is not a 32-byte hex string")
	}
	if artefact.ClaimDigest != "" && !isHex(artefact.ClaimDigest, 32) {
		return nil, errors.New("the artefact's claim digest is not a 32-byte hex string")
	}

	// The escrow verifies the seal against the claim the pinned guest program
	// produces for this journal, so a receipt from another program - or one whose
	// journal was altered after it was proved - can never register. Both are
	// refused here rather than sent to a node that answers `invalid_proof`.
	if !strings.EqualFold(strings.TrimPrefix(artefact.ImageID, "0x"), expectedImage) {
		if artefact.ImageID == "" {
			return nil, errors.New("the artefact records no image id, so the call cannot be tied to the release's guest program")
		}
		return nil, fmt.Errorf(
			"the receipt was proved by guest program %s, and this release pins %s",
			strings.TrimPrefix(artefact.ImageID, "0x"), expectedImage)
	}
	expected, err := claimDigestFromHex(expectedImage, journal)
	if err != nil {
		return nil, fmt.Errorf("compute the claim the escrow checks: %w", err)
	}
	recorded, err := hex.DecodeString(strings.TrimPrefix(artefact.ClaimDigest, "0x"))
	if err != nil || len(recorded) != 32 {
		return nil, errors.New("the artefact records no claim digest, so the journal cannot be tied to the seal it came with")
	}
	if !equalBytes(recorded, expected[:]) {
		return nil, fmt.Errorf(
			"the artefact's claim digest %s is not the claim this journal and the pinned guest program produce (%x): the receipt and its journal do not belong together",
			strings.TrimPrefix(artefact.ClaimDigest, "0x"), expected)
	}

	calldata := make([]byte, 0, len(journal)+len(seal))
	calldata = append(calldata, journal...)
	calldata = append(calldata, seal...)

	return &PreparedRegistration{
		Format:             format,
		Version:            actionRequestVersion,
		Operation:          operation,
		Network:            network,
		Contract:           contract,
		Author:             author,
		Epoch:              epoch,
		CoveredFlips:       covered,
		ManifestCommitment: hex.EncodeToString(journal[registrationManifestOffset : registrationManifestOffset+32]),
		KeyCommitment:      hex.EncodeToString(journal[registrationCommitmentOffset : registrationCommitmentOffset+32]),
		InventoryCommitment: hex.EncodeToString(
			journal[registrationInventoryOffset : registrationInventoryOffset+32],
		),
		PolicyDigest:    policy,
		ConsensusDigest: consensus,
		NetworkDigest:   networkDigest,
		ImageID:         strings.ToLower(strings.TrimPrefix(artefact.ImageID, "0x")),
		ClaimDigest:     strings.ToLower(strings.TrimPrefix(artefact.ClaimDigest, "0x")),
		Calldata:        hex.EncodeToString(calldata),
		// Registration sends no value: the bundle is recorded, not paid for.
		Amount:     "0",
		Proof:      proofPath,
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
		Submitted:  false,
	}, nil
}

// runPrepareRegistration validates a sealed receipt and writes the pool's
// `registerBundle` call.
func runPrepareRegistration(proofPath, expectedContract, network, preparedPath string) {
	artefact, err := readRegistrationArtefact(proofPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	prepared, err := AssemblePoolRegistration(network, expectedContract, proofPath, artefact)
	if err != nil {
		fmt.Fprintf(os.Stderr, "refused the registration artefact: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("operation   %s on %s\n", prepared.Operation, prepared.Network)
	fmt.Printf("contract    %s\n", prepared.Contract)
	fmt.Printf("bundle      %s\n", prepared.KeyCommitment)
	fmt.Printf("author      %s (epoch %d)\n", prepared.Author, prepared.Epoch)
	fmt.Printf("covered     %d flips, manifest %s\n", prepared.CoveredFlips, prepared.ManifestCommitment)
	if prepared.ImageID != "" {
		fmt.Printf("image id    %s\n", prepared.ImageID)
	}
	if prepared.ClaimDigest != "" {
		fmt.Printf("claim       %s\n", prepared.ClaimDigest)
	}
	fmt.Printf("calldata    %d bytes\n", len(prepared.Calldata)/2)
	if preparedPath != "" {
		if err := writeJSON(preparedPath, prepared); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", preparedPath, err)
			os.Exit(1)
		}
		fmt.Printf("prepared    %s\n", preparedPath)
	}
	notSubmitted(preparedPath, "only the author the receipt names can send it")
}
