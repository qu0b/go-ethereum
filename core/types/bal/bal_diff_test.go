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
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

func acct(addr common.Address, reads ...uint64) AccountAccess {
	a := AccountAccess{Address: addr}
	for _, r := range reads {
		a.StorageReads = append(a.StorageReads, uint256.NewInt(r))
	}
	return a
}

// A builder can win a slot with a payload whose BAL claims a storage read that
// execution never performs. Every client rejects it, but only on the hash, so
// the operator cannot tell which entry is wrong.
func TestDiffNamesAPhantomStorageRead(t *testing.T) {
	victim := common.HexToAddress("0xba1ba1ba1ba1ba1ba1ba1ba1ba1ba1ba1ba1ba1b")

	local := BlockAccessList{acct(victim, 1)}
	remote := BlockAccessList{acct(victim, 1, 42)}

	diff := local.Diff(remote)
	if !strings.Contains(diff, victim.String()) {
		t.Fatalf("diff does not name the account: %s", diff)
	}
	if !strings.Contains(diff, "0x000000000000000000000000000000000000000000000000000000000000002a") {
		t.Fatalf("diff does not name the offending slot: %s", diff)
	}
	if !strings.Contains(diff, "never happened locally") {
		t.Fatalf("diff does not say the entry is unjustified: %s", diff)
	}
}

func TestDiffNamesAMissingLocalRead(t *testing.T) {
	victim := common.HexToAddress("0x0000000000000000000000000000000000001234")

	local := BlockAccessList{acct(victim, 7)}
	remote := BlockAccessList{acct(victim)}

	diff := local.Diff(remote)
	if !strings.Contains(diff, "0x0000000000000000000000000000000000000000000000000000000000000007") {
		t.Fatalf("diff does not name the slot: %s", diff)
	}
	if !strings.Contains(diff, "present locally but not in the block") {
		t.Fatalf("diff does not describe the direction: %s", diff)
	}
}

func TestDiffNamesAccountsPresentOnOneSideOnly(t *testing.T) {
	a := common.HexToAddress("0x0000000000000000000000000000000000000001")
	b := common.HexToAddress("0x0000000000000000000000000000000000000002")

	diff := BlockAccessList{acct(a)}.Diff(BlockAccessList{acct(b)})
	if !strings.Contains(diff, "local only") || !strings.Contains(diff, "remote only") {
		t.Fatalf("diff does not report both directions: %s", diff)
	}
}

func TestDiffIsHonestWhenItFindsNothing(t *testing.T) {
	a := common.HexToAddress("0x0000000000000000000000000000000000000001")

	diff := BlockAccessList{acct(a, 1)}.Diff(BlockAccessList{acct(a, 1)})
	if !strings.Contains(diff, "no per-account difference found") {
		t.Fatalf("diff overclaims: %s", diff)
	}
}
