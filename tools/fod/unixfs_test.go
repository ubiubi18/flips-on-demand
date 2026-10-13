package main

// The fixtures are CIDs a live mainnet node answered for these exact bytes
// through `ipfs_cid`, recorded in `SOURCES.md` F-88. The bytes are generated
// here rather than stored, because a CID depends on them exactly.

import "testing"

// nodeCIDPattern is the deterministic byte pattern the node was asked about.
func nodeCIDPattern(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte((i*31 + 7) % 251)
	}
	return out
}

func TestUnixFSCIDMatchesTheNodesOwnImporter(t *testing.T) {
	cases := []struct {
		size int
		cid  string
	}{
		{262144, "bafkreieiuj5mzeuqor24l4lmo2yumm6665yv4gq7ozzp2nrfmqrjyboosy"},
		{262145, "bafybeich6cqd6yabw3cku6edwmz4rvwe275nx2l6ootvc2bpbt5bdvarua"},
		{524288, "bafybeid5sthfzhlnifk5fvivasemgw2rzzfdp54rapknx7eaz5j4yoae3i"},
		{524289, "bafybeiag5snf6cdu2gfoobd6elsfvq3tsujp3gztgpvulhyzxo3ax6e2vy"},
		{614400, "bafybeihr2fddcxgyaodxyumttzp2zioqpjezbsynz7v3rwsrh3ogrhnliy"},
	}
	for _, item := range cases {
		blob := nodeCIDPattern(item.size)
		computed, err := unixfsCID(blob)
		if err != nil {
			t.Fatalf("%d bytes: %v", item.size, err)
		}
		if computed != item.cid {
			t.Fatalf("%d bytes: computed %s, the node answered %s", item.size, computed, item.cid)
		}
		// The buyer's own check has to accept these bytes without asking any
		// node, and the peer's `ipfs_cid` answer must stay unused: a substituted
		// blob plus a lying endpoint cannot become verified content
		// (`docs/PROGRESS.md`, the CID fallback finding).
		asked := 0
		if ok, note := verifyBlobCID(item.cid, blob, func([]byte) (string, error) {
			asked++
			return item.cid, nil
		}); !ok {
			t.Fatalf("%d bytes: the locally reproduced cid %s was refused: %s", item.size, item.cid, note)
		}
		if asked != 0 {
			t.Fatalf("%d bytes: the serving peer was asked to authorize its own bytes", item.size)
		}
	}
}

func TestUnixFSRefusesABlobItWouldNeedAnotherLevelFor(t *testing.T) {
	if _, err := unixfsCID(nodeCIDPattern(unixfsMaxLinks*unixfsChunkSize + 1)); err == nil {
		t.Fatal("a blob that needs a second dag-pb level was accepted as a single root")
	}
}
