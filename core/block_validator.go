// Copyright 2015 The go-ethereum Authors
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
	"errors"
	"fmt"

	"github.com/antithesishq/antithesis-sdk-go/assert"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
)

// BlockValidator is responsible for validating block headers, uncles and
// processed state.
//
// BlockValidator implements Validator.
type BlockValidator struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
}

// NewBlockValidator returns a new block validator which is safe for re-use
func NewBlockValidator(config *params.ChainConfig, blockchain *BlockChain) *BlockValidator {
	validator := &BlockValidator{
		config: config,
		bc:     blockchain,
	}
	return validator
}

// ValidateBody validates the given block's uncles and verifies the block
// header's transaction and uncle roots. The headers are assumed to be already
// validated at this point.
func (v *BlockValidator) ValidateBody(block *types.Block) error {
	// Assert that block is not nil
	assert.Always(block != nil, "Block must not be nil", nil)

	// Check whether the block is already imported.
	if v.bc.HasBlockAndState(block.Hash(), block.NumberU64()) {
		// Assert that we have seen this block before
		assert.Sometimes(true, "Block has been seen before", map[string]any{"blockHash": block.Hash()})
		return ErrKnownBlock
	} else {
		// Assert that block is new
		assert.Sometimes(true, "Block is new", map[string]any{"blockHash": block.Hash()})
	}

	// Header validity is known at this point. Here we verify that uncles, transactions
	// and withdrawals given in the block body match the header.
	header := block.Header()
	assert.Always(header != nil, "Block header must not be nil", nil)

	// Verify uncles
	if err := v.bc.engine.VerifyUncles(v.bc, block); err != nil {
		// Assert that uncles sometimes fail verification
		assert.Sometimes(true, "Uncles failed verification", map[string]any{"error": err.Error()})
		return err
	} else {
		// Assert that uncles pass verification
		assert.Sometimes(true, "Uncles passed verification", nil)
	}

	// Verify uncle hash
	if hash := types.CalcUncleHash(block.Uncles()); hash != header.UncleHash {
		// Assert that uncle hash mismatch occurs
		assert.Sometimes(true, "Uncle hash mismatch", map[string]any{"expected": header.UncleHash, "calculated": hash})
		return fmt.Errorf("uncle root hash mismatch (header value %x, calculated %x)", header.UncleHash, hash)
	} else {
		// Assert that uncle hash matches
		assert.Always(hash == header.UncleHash, "Uncle hash matches header", nil)
	}

	// Verify transaction hash
	if hash := types.DeriveSha(block.Transactions(), trie.NewStackTrie(nil)); hash != header.TxHash {
		// Assert that transaction root hash mismatch occurs
		assert.Sometimes(true, "Transaction root hash mismatch", map[string]any{"expected": header.TxHash, "calculated": hash})
		return fmt.Errorf("transaction root hash mismatch (header value %x, calculated %x)", header.TxHash, hash)
	} else {
		// Assert that transaction root hash matches
		assert.Always(hash == header.TxHash, "Transaction root hash matches header", nil)
	}

	// Withdrawals are present after the Shanghai fork.
	if header.WithdrawalsHash != nil {
		// Withdrawals list must be present in body after Shanghai.
		if block.Withdrawals() == nil {
			// Assert that withdrawals are missing when withdrawals hash is present
			assert.Sometimes(true, "Missing withdrawals in block body", nil)
			return errors.New("missing withdrawals in block body")
		}
		if hash := types.DeriveSha(block.Withdrawals(), trie.NewStackTrie(nil)); hash != *header.WithdrawalsHash {
			// Assert that withdrawals root hash mismatch occurs
			assert.Sometimes(true, "Withdrawals root hash mismatch", map[string]any{"expected": *header.WithdrawalsHash, "calculated": hash})
			return fmt.Errorf("withdrawals root hash mismatch (header value %x, calculated %x)", *header.WithdrawalsHash, hash)
		} else {
			// Assert that withdrawals root hash matches
			assert.Always(hash == *header.WithdrawalsHash, "Withdrawals root hash matches header", nil)
		}
	} else if block.Withdrawals() != nil {
		// Withdrawals are not allowed prior to Shanghai fork
		// Assert that withdrawals are present when withdrawals hash is nil
		assert.Sometimes(true, "Withdrawals present in block body before Shanghai", nil)
		return errors.New("withdrawals present in block body")
	}

	// Blob transactions may be present after the Cancun fork.
	var blobs int
	for i, tx := range block.Transactions() {
		// Count the number of blobs to validate against the header's blobGasUsed
		blobs += len(tx.BlobHashes())

		// If the tx is a blob tx, it must NOT have a sidecar attached to be valid in a block.
		if tx.BlobTxSidecar() != nil {
			// Assert that unexpected blob sidecar is present
			assert.Sometimes(true, "Unexpected blob sidecar in transaction", map[string]any{"txIndex": i})
			return fmt.Errorf("unexpected blob sidecar in transaction at index %d", i)
		}

		// The individual checks for blob validity (version-check + not empty)
		// happens in StateTransition.
	}

	// Check blob gas usage.
	if header.BlobGasUsed != nil {
		if want := *header.BlobGasUsed / params.BlobTxBlobGasPerBlob; uint64(blobs) != want {
			// Assert that blob gas used mismatch occurs
			assert.Sometimes(true, "Blob gas used mismatch", map[string]any{"expected": want * params.BlobTxBlobGasPerBlob, "calculated": blobs * params.BlobTxBlobGasPerBlob})
			return fmt.Errorf("blob gas used mismatch (header %v, calculated %v)", *header.BlobGasUsed, blobs*params.BlobTxBlobGasPerBlob)
		} else {
			// Assert that blob gas used matches
			assert.Always(uint64(blobs) == want, "Blob gas used matches header", nil)
		}
	} else {
		if blobs > 0 {
			// Assert that data blobs are present when BlobGasUsed is nil
			assert.Sometimes(true, "Data blobs present in block body before Cancun", nil)
			return errors.New("data blobs present in block body")
		}
	}

	// Ancestor block must be known.
	if !v.bc.HasBlockAndState(block.ParentHash(), block.NumberU64()-1) {
		// Assert that ancestor block is missing
		assert.Sometimes(true, "Ancestor block is missing", map[string]any{"parentHash": block.ParentHash()})
		if !v.bc.HasBlock(block.ParentHash(), block.NumberU64()-1) {
			return consensus.ErrUnknownAncestor
		}
		return consensus.ErrPrunedAncestor
	} else {
		// Assert that ancestor block is known
		assert.Always(true, "Ancestor block is known", nil)
	}
	return nil
}

// ValidateState validates the various changes that happen after a state transition,
// such as amount of used gas, the receipt roots and the state root itself.
func (v *BlockValidator) ValidateState(block *types.Block, statedb *state.StateDB, res *ProcessResult, stateless bool) error {
	if res == nil {
		// Assert that ProcessResult is nil
		assert.Sometimes(true, "ProcessResult is nil", nil)
		return fmt.Errorf("nil ProcessResult value")
	} else {
		// Assert that ProcessResult is valid
		assert.Always(true, "ProcessResult is valid", nil)
	}
	header := block.Header()
	if block.GasUsed() != res.GasUsed {
		// Assert that gas used mismatch occurs
		assert.Sometimes(true, "Gas used mismatch", map[string]any{"blockGasUsed": block.GasUsed(), "resultGasUsed": res.GasUsed})
		return fmt.Errorf("invalid gas used (remote: %d local: %d)", block.GasUsed(), res.GasUsed)
	} else {
		// Assert that gas used matches
		assert.Always(block.GasUsed() == res.GasUsed, "Gas used matches", nil)
	}
	// Validate the received block's bloom with the one derived from the generated receipts.
	// For valid blocks this should always validate to true.
	rbloom := types.CreateBloom(res.Receipts)
	if rbloom != header.Bloom {
		// Assert that bloom filter mismatch occurs
		assert.Sometimes(true, "Bloom filter mismatch", map[string]any{"headerBloom": header.Bloom, "calculatedBloom": rbloom})
		return fmt.Errorf("invalid bloom (remote: %x  local: %x)", header.Bloom, rbloom)
	} else {
		// Assert that bloom filter matches
		assert.Always(rbloom == header.Bloom, "Bloom filter matches", nil)
	}
	// In stateless mode, return early because the receipt and state root are not
	// provided through the witness, rather the cross validator needs to return it.
	if stateless {
		return nil
	}
	// The receipt Trie's root (R = (Tr [[H1, R1], ... [Hn, Rn]]))
	receiptSha := types.DeriveSha(res.Receipts, trie.NewStackTrie(nil))
	if receiptSha != header.ReceiptHash {
		// Assert that receipt root hash mismatch occurs
		assert.Sometimes(true, "Receipt root hash mismatch", map[string]any{"expected": header.ReceiptHash, "calculated": receiptSha})
		return fmt.Errorf("invalid receipt root hash (remote: %x local: %x)", header.ReceiptHash, receiptSha)
	} else {
		// Assert that receipt root hash matches
		assert.Always(receiptSha == header.ReceiptHash, "Receipt root hash matches", nil)
	}
	// Validate the parsed requests match the expected header value.
	if header.RequestsHash != nil {
		depositSha := types.DeriveSha(res.Requests, trie.NewStackTrie(nil))
		if depositSha != *header.RequestsHash {
			// Assert that deposit root hash mismatch occurs
			assert.Sometimes(true, "Deposit root hash mismatch", map[string]any{"expected": *header.RequestsHash, "calculated": depositSha})
			return fmt.Errorf("invalid deposit root hash (remote: %x local: %x)", *header.RequestsHash, depositSha)
		} else {
			// Assert that deposit root hash matches
			assert.Always(depositSha == *header.RequestsHash, "Deposit root hash matches", nil)
		}
	}
	// Validate the state root against the received state root and throw
	// an error if they don't match.
	if root := statedb.IntermediateRoot(v.config.IsEIP158(header.Number)); header.Root != root {
		// Assert that state root mismatch occurs
		assert.Sometimes(true, "State root mismatch", map[string]any{"expected": header.Root, "calculated": root, "dbError": statedb.Error()})
		return fmt.Errorf("invalid merkle root (remote: %x local: %x) dberr: %w", header.Root, root, statedb.Error())
	} else {
		// Assert that state root matches
		assert.Always(header.Root == root, "State root matches", nil)
	}
	return nil
}

// CalcGasLimit computes the gas limit of the next block after parent. It aims
// to keep the baseline gas close to the provided target, and increase it towards
// the target if the baseline gas is lower.
func CalcGasLimit(parentGasLimit, desiredLimit uint64) uint64 {
	delta := parentGasLimit/params.GasLimitBoundDivisor - 1
	limit := parentGasLimit
	if desiredLimit < params.MinGasLimit {
		desiredLimit = params.MinGasLimit
	}
	// If we're outside our allowed gas range, we try to hone towards them
	if limit < desiredLimit {
		limit = parentGasLimit + delta
		if limit > desiredLimit {
			limit = desiredLimit
		}
		// Assert that gas limit increases towards desired limit
		assert.Sometimes(true, "Gas limit increases towards desired limit", nil)
		return limit
	}
	if limit > desiredLimit {
		limit = parentGasLimit - delta
		if limit < desiredLimit {
			limit = desiredLimit
		}
		// Assert that gas limit decreases towards desired limit
		assert.Sometimes(true, "Gas limit decreases towards desired limit", nil)
	}
	// Assert that gas limit is within allowed range
	assert.Always(limit >= params.MinGasLimit, "Gas limit is above minimum", nil)
	return limit
}