// Command keyscan lists the epoch-prefixed keys of an Idena chain database.
//
// One-off: written for a check on a host that has no Go toolchain, so it imports
// only goleveldb and cross-compiles without cgo, where `fod epoch-keys` needs the
// node's crypto. The key layout is the node's own (database/epoch_db.go):
// "epoch" + two big-endian bytes, then a sub-key such as "cid", "li" (lottery
// identities) or "ls" (lottery seed).
//
// The database is opened read-only, so a node's datadir is never modified.
// It was validated against the local datadir whose contents `SOURCES.md` F-34
// records - one epoch, `cid` only, 32 keys - before being pointed at any other.
package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"sort"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var knownPrefixes = []string{"s-tx", "pubk", "hash", "evi", "cid", "tx", "pk", "li", "ls"}

func classify(sub []byte) string {
	for _, prefix := range knownPrefixes {
		if len(sub) >= len(prefix) && string(sub[:len(prefix)]) == prefix {
			return prefix
		}
	}
	if len(sub) > 4 {
		return "other:" + hex.EncodeToString(sub[:4])
	}
	return "other:" + hex.EncodeToString(sub)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: keyscan <datadir> [datadir...]")
		os.Exit(2)
	}
	for _, dir := range os.Args[1:] {
		scan(dir)
	}
}

func scan(dir string) {
	path := dir + "/idenachain.db"
	db, err := leveldb.OpenFile(path, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		fmt.Printf("%s\n  cannot open: %v\n", dir, err)
		return
	}
	defer db.Close()

	counts := map[uint16]map[string]int{}
	iterator := db.NewIterator(util.BytesPrefix([]byte("epoch")), nil)
	for iterator.Next() {
		key := iterator.Key()
		if len(key) < 8 {
			continue
		}
		epoch := binary.BigEndian.Uint16(key[5:7])
		name := classify(key[7:])
		if counts[epoch] == nil {
			counts[epoch] = map[string]int{}
		}
		counts[epoch][name]++
	}
	err = iterator.Error()
	iterator.Release()
	if err != nil {
		fmt.Printf("%s\n  iterate: %v\n", dir, err)
		return
	}

	epochs := make([]int, 0, len(counts))
	for epoch := range counts {
		epochs = append(epochs, int(epoch))
	}
	sort.Ints(epochs)

	lottery := 0
	fmt.Printf("%s\n  %d epochs with keys\n", dir, len(epochs))
	for _, epoch := range epochs {
		subs := make([]string, 0, len(counts[uint16(epoch)]))
		for sub := range counts[uint16(epoch)] {
			subs = append(subs, sub)
		}
		sort.Strings(subs)
		fmt.Printf("  epoch %d:", epoch)
		for _, sub := range subs {
			fmt.Printf(" %s=%d", sub, counts[uint16(epoch)][sub])
			if sub == "li" || sub == "ls" {
				lottery++
			}
		}
		fmt.Println()
	}
	if lottery > 0 {
		fmt.Println("  LOTTERY RECORDS PRESENT")
	}
}
