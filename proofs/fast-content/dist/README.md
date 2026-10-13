# Synthetic fast-content receipt

`synthetic-one-flip.json` is a genuine Groth16 receipt from the pinned
`fod:fast-content:v1` guest. It uses the frozen synthetic flip vector and a
placeholder contract address. It contains only the public journal, seal,
claim/image identifiers, and original/copy CID pair; no authoring witness or
seller key is included. The host verified the receipt before writing it, and
`TestFastContentSealVerifiesInPinnedRuntime` verifies its seal in the Idena
contract runtime.

The fixture cannot register a live identity or contract. Each live author
needs a separate proof bound to the deployed contract and their original
flips, followed by a canonical four-leaf session witness.

`synthetic-v5-one-flip.json` is a second genuine sealed receipt for the same
original synthetic CID. It pins the later v5 guest image without changing the
image accepted by the deployed v4 fast escrow. It likewise contains only the
public journal, seal, claim/image identifiers, and original/copy CID pair.
