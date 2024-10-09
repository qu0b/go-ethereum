// Copyright 2019 The go-ethereum Authors
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

package core

import (
	"sync/atomic"

	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/antithesishq/antithesis-sdk-go/assert"
)

// statePrefetcher is a basic Prefetcher, which blindly executes a block on top
// of an arbitrary state with the goal of prefetching potentially useful state
// data from disk before the main block processor start executing.
type statePrefetcher struct {
	config *params.ChainConfig // Chain configuration options
	chain  *HeaderChain        // Canonical block chain
}

// newStatePrefetcher initialises a new statePrefetcher.
func newStatePrefetcher(config *params.ChainConfig, chain *HeaderChain) *statePrefetcher {
	return &statePrefetcher{
		config: config,
		chain:  chain,
	}
}

// Prefetch processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb, but any changes are discarded. The
// only goal is to pre-cache transaction signatures and state trie nodes.
func (p *statePrefetcher) Prefetch(block *types.Block, statedb *state.StateDB, cfg vm.Config, interrupt *atomic.Bool) {
	assert.Always(block != nil, "Block should not be nil", nil)
	assert.Always(statedb != nil, "StateDB should not be nil", nil)
	assert.Sometimes(len(block.Transactions()) == 0, "Processing block with zero transactions", nil)
	assert.Sometimes(len(block.Transactions()) > 0, "Processing block with transactions", nil)
	assert.Sometimes(block.NumberU64() == 0, "Processing genesis block", nil)
	assert.Sometimes(block.NumberU64() > 0, "Processing non-genesis block", nil)

	var (
		header       = block.Header()
		gaspool      = new(GasPool).AddGas(block.GasLimit())
		blockContext = NewEVMBlockContext(header, p.chain, nil)
		evm          = vm.NewEVM(blockContext, vm.TxContext{}, statedb, p.config, cfg)
		signer       = types.MakeSigner(p.config, header.Number, header.Time)
	)
	assert.Always(header != nil, "Block header should not be nil", nil)
	// Iterate over and process the individual transactions
	byzantium := p.config.IsByzantium(block.Number())
	assert.Sometimes(byzantium, "Block is Byzantium", map[string]any{"block": block.Number()})
	assert.Sometimes(!byzantium, "Block is pre-Byzantium", map[string]any{"block": block.Number()})
	if len(block.Transactions()) == 0 {
		assert.Reachable("Block has zero transactions", nil)
	}
	for i, tx := range block.Transactions() {
		assert.Always(tx != nil, "Transaction should not be nil", nil)
		// If block precaching was interrupted, abort
		if interrupt != nil && interrupt.Load() {
			assert.Sometimes(true, "Prefetch interrupted during transaction processing", nil)
			return
		}
		// Convert the transaction into an executable message and pre-cache its sender
		msg, err := TransactionToMessage(tx, signer, header.BaseFee)
		if err != nil {
			assert.Sometimes(true, "TransactionToMessage returned an error", map[string]any{"error": err})
			return // Also invalid block, bail out
		}
		statedb.SetTxContext(tx.Hash(), i)
		err = precacheTransaction(msg, p.config, gaspool, statedb, header, evm)
		if err != nil {
			assert.Sometimes(true, "PrecacheTransaction returned an error", map[string]any{"error": err})
			return // Ugh, something went horribly wrong, bail out
		}
		// If we're pre-byzantium, pre-load trie nodes for the intermediate root
		if !byzantium {
			assert.Sometimes(true, "Processing pre-Byzantium block", nil)
			statedb.IntermediateRoot(true)
		}
	}
	// If were post-byzantium, pre-load trie nodes for the final root hash
	if byzantium {
		assert.Sometimes(true, "Processing post-Byzantium block", nil)
		statedb.IntermediateRoot(true)
	}
	assert.Sometimes(true, "Reached end of Prefetch function", nil)
}

// precacheTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. The goal is not to execute
// the transaction successfully, rather to warm up touched data slots.
func precacheTransaction(msg *Message, config *params.ChainConfig, gaspool *GasPool, statedb *state.StateDB, header *types.Header, evm *vm.EVM) error {
	assert.Always(msg != nil, "Message should not be nil", nil)
	assert.Always(evm != nil, "EVM should not be nil", nil)
	// Update the evm with the new transaction context.
	evm.Reset(NewEVMTxContext(msg), statedb)
	// Add addresses to access list if applicable
	_, err := ApplyMessage(evm, msg, gaspool)
	assert.Sometimes(err != nil, "ApplyMessage returned an error", map[string]any{"error": err})
	return err
}