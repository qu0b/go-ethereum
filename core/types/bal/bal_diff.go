// Copyright 2026 The go-ethereum Authors
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

package bal

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

// Diff describes the first place two block access lists disagree.
//
// A BAL hash mismatch reports two 32-byte hashes and nothing else, which says
// that the lists differ but not how. Recovering the offending entry from that
// means re-deriving the whole list by hand. Diff answers the question directly
// so a mismatch can be triaged from the log line.
func (e BlockAccessList) Diff(other BlockAccessList) string {
	var b strings.Builder

	local := index(e)
	remote := index(other)

	// Accounts present on one side only.
	for _, addr := range sortedAddrs(local) {
		if _, ok := remote[addr]; !ok {
			fmt.Fprintf(&b, "account %s: local only; ", addr)
		}
	}
	for _, addr := range sortedAddrs(remote) {
		if _, ok := local[addr]; !ok {
			fmt.Fprintf(&b, "account %s: remote only; ", addr)
		}
	}

	// Accounts on both sides, compared field by field.
	for _, addr := range sortedAddrs(local) {
		r, ok := remote[addr]
		if !ok {
			continue
		}
		l := local[addr]
		diffSlots(&b, addr, "storage_change", slotsOf(l.StorageChanges), slotsOf(r.StorageChanges))
		diffSlots(&b, addr, "storage_read", l.StorageReads, r.StorageReads)
		if len(l.BalanceChanges) != len(r.BalanceChanges) {
			fmt.Fprintf(&b, "account %s: balance_changes local %d, remote %d; ", addr, len(l.BalanceChanges), len(r.BalanceChanges))
		}
		if len(l.NonceChanges) != len(r.NonceChanges) {
			fmt.Fprintf(&b, "account %s: nonce_changes local %d, remote %d; ", addr, len(l.NonceChanges), len(r.NonceChanges))
		}
		if len(l.CodeChanges) != len(r.CodeChanges) {
			fmt.Fprintf(&b, "account %s: code_changes local %d, remote %d; ", addr, len(l.CodeChanges), len(r.CodeChanges))
		}
	}

	if b.Len() == 0 {
		// Same entries in a different order, or a difference in a field this
		// summary does not walk. Say so rather than claiming they match.
		return fmt.Sprintf("no per-account difference found (local %d accounts, remote %d); lists differ in ordering or in per-transaction values", len(e), len(other))
	}
	return strings.TrimSuffix(b.String(), "; ")
}

// diffSlots reports slots present on one side only, for one kind of slot list.
func diffSlots(b *strings.Builder, addr common.Address, kind string, local, remote []*uint256.Int) {
	l := slotSet(local)
	r := slotSet(remote)
	for _, slot := range sortedSlots(l) {
		if _, ok := r[slot]; !ok {
			fmt.Fprintf(b, "account %s: %s for slot %#x present locally but not in the block; ", addr, kind, slot)
		}
	}
	for _, slot := range sortedSlots(r) {
		if _, ok := l[slot]; !ok {
			fmt.Fprintf(b, "account %s: %s for slot %#x present in the block but never happened locally; ", addr, kind, slot)
		}
	}
}

func index(list BlockAccessList) map[common.Address]AccountAccess {
	out := make(map[common.Address]AccountAccess, len(list))
	for _, entry := range list {
		out[entry.Address] = entry
	}
	return out
}

func slotsOf(changes []encodingSlotChanges) []*uint256.Int {
	out := make([]*uint256.Int, 0, len(changes))
	for i := range changes {
		out = append(out, changes[i].Slot)
	}
	return out
}

func slotSet(slots []*uint256.Int) map[common.Hash]struct{} {
	out := make(map[common.Hash]struct{}, len(slots))
	for _, slot := range slots {
		if slot != nil {
			out[common.Hash(slot.Bytes32())] = struct{}{}
		}
	}
	return out
}

func sortedAddrs(m map[common.Address]AccountAccess) []common.Address {
	out := make([]common.Address, 0, len(m))
	for addr := range m {
		out = append(out, addr)
	}
	slices.SortFunc(out, func(a, b common.Address) int { return bytes.Compare(a[:], b[:]) })
	return out
}

func sortedSlots(m map[common.Hash]struct{}) []common.Hash {
	out := make([]common.Hash, 0, len(m))
	for slot := range m {
		out = append(out, slot)
	}
	slices.SortFunc(out, func(a, b common.Hash) int { return bytes.Compare(a[:], b[:]) })
	return out
}
