// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package txpool

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestLess(t *testing.T) {
	// a > b
	// a < c
	// c < b
	key, _ := crypto.GenerateKey()
	a := createTxEntry(0, 12, big.NewInt(10), key)
	b := createTxEntry(1, 14, big.NewInt(14), key)
	if a.Less(b) {
		t.Fatal("a less than b")
	}
	if !b.Less(a) {
		t.Fatal("b not less than a")
	}

	key2, _ := crypto.GenerateKey()
	c := createTxEntry(0, 13, big.NewInt(13), key2)
	if !a.Less(c) {
		t.Fatal("a not less than c")
	}
	if c.Less(a) {
		t.Fatal("c less than a")
	}
	if b.Less(c) {
		t.Fatal("b less than c")
	}
	if !c.Less(b) {
		t.Fatal("c not less than b")
	}
}

func TestTxList(t *testing.T) {
	txlist := newTxList(10)
	key, _ := crypto.GenerateKey()
	txs := []*txEntry{
		createTxEntry(0, 12, big.NewInt(12), key),
		createTxEntry(1, 13, big.NewInt(13), key),
		createTxEntry(2, 10, big.NewInt(10), key),
		createTxEntry(3, 14, big.NewInt(14), key),
	}

	for _, tx := range txs {
		if txlist.Add(tx) {
			t.Fatal("Add returned shouldPrune = true, wanted false")
		}
	}
	if txlist.Len() != 4 {
		t.Fatalf("Invalid length %v, want %v", txlist.Len(), 4)
	}
	printTxList(txlist)
	// Retrieve last entry
	last := txlist.LastEntry()
	if last.tx != txs[3].tx {
		t.Fatalf("LastEntry returned false entry %v, want %v", last.tx.Nonce(), txs[3].tx.Nonce())
	}
	// Delete second transactions
	entry := txlist.Delete(func(e *txEntry) bool {
		return e.sender == txs[2].sender && e.tx.Nonce() == txs[2].tx.Nonce()
	})
	if entry == nil {
		t.Fatal("No entry found")
	}
	if entry.tx != txs[2].tx {
		t.Fatalf("Wrong tx retrieved got %v, want %v", entry.tx.Nonce(), txs[2].tx.Nonce())
	}
	if txlist.Len() != 3 {
		t.Fatalf("Invalid length %v, want %v", txlist.Len(), 3)
	}
	// Peek 5 transactions
	peeked := txlist.Peek(5)
	if len(peeked) != 3 {
		t.Fatalf("Invalid amount of txs peeked got %v, want %v", len(peeked), 3)
	}
	if peeked[0] != txs[0].tx {
		t.Fatalf("Wrong tx retrieved got %v, want %v", peeked[0].Nonce(), txs[0].tx.Nonce())
	}
	if peeked[1] != txs[1].tx {
		t.Fatalf("Wrong tx retrieved got %v, want %v", peeked[1].Nonce(), txs[1].tx.Nonce())
	}
	if peeked[2] != txs[3].tx {
		t.Fatalf("Wrong tx retrieved got %v, want %v", peeked[2].Nonce(), txs[3].tx.Nonce())
	}
	// Add two transactions, one at the top, one at the bottom
	key2, _ := crypto.GenerateKey()
	txs2 := []*txEntry{
		createTxEntry(0, 12, big.NewInt(1000), key2),
		createTxEntry(0, 12, big.NewInt(1), key2),
	}
	for _, tx := range txs2 {
		if txlist.Add(tx) {
			t.Fatal("Add returned shouldPrune = true, wanted false")
		}
	}
	if txlist.Len() != 5 {
		t.Fatalf("Invalid length %v, want %v", txlist.Len(), 5)
	}
	// Peek 5 transactions
	peeked = txlist.Peek(5)
	if len(peeked) != 5 {
		t.Fatalf("Invalid amount of txs peeked got %v, want %v", len(peeked), 5)
	}
	if peeked[0] != txs2[0].tx {
		t.Fatalf("Wrong tx retrieved got %v, want %v", peeked[0].Nonce(), txs2[0].tx.Nonce())
	}
	if peeked[1] != txs[0].tx {
		t.Fatalf("Wrong tx retrieved got %v, want %v", peeked[1].Nonce(), txs[0].tx.Nonce())
	}
	if peeked[2] != txs[1].tx {
		t.Fatalf("Wrong tx retrieved got %v, want %v", peeked[2].Nonce(), txs[1].tx.Nonce())
	}
	if peeked[3] != txs[3].tx {
		t.Fatalf("Wrong tx retrieved got %v, want %v", peeked[3].Nonce(), txs[3].tx.Nonce())
	}
	if peeked[4] != txs2[1].tx {
		t.Fatalf("Wrong tx retrieved got %v, want %v", peeked[4].Nonce(), txs2[1].tx.Nonce())
	}
}

func createTxEntry(nonce, gaslimit uint64, gasprice *big.Int, key *ecdsa.PrivateKey) *txEntry {
	tx := pricedTransaction(nonce, gaslimit, gasprice, key)
	sender, err := types.Sender(types.HomesteadSigner{}, tx)
	if err != nil {
		panic(err)
	}
	return &txEntry{tx: tx, sender: sender, price: tx.GasPrice()}
}

func printTxList(l txList) {
	i := 0
	for new := l.head; new != nil; new = new.next {
		fmt.Printf("%v: %v\n", i, new.tx.Nonce())
		i++
	}
}
