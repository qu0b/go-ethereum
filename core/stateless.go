package core

import (
	"github.com/antithesishq/antithesis-sdk-go/assert"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/lru"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/ethereum/go-ethereum/triedb"
	"time"
)

func ExecuteStateless(config *params.ChainConfig, block *types.Block, witness *stateless.Witness) (common.Hash, common.Hash, error) {
	if block.Root() != (common.Hash{}) {
		log.Error("stateless runner received state root it's expected to calculate (faulty consensus client)", "block", block.Number())
	}
	assert.Always(block.Root() == (common.Hash{}), "Block root should be empty at start", nil)
	if block.ReceiptHash() != (common.Hash{}) {
		log.Error("stateless runner received receipt root it's expected to calculate (faulty consensus client)", "block", block.Number())
	}
	assert.Always(block.ReceiptHash() == (common.Hash{}), "Receipt hash should be empty at start", nil)
	currentTime := uint64(time.Now().Unix())
	assert.Always(block.Time() <= currentTime, "Block timestamp should not be in the future", nil)
	memdb := witness.MakeHashDB()
	db, err := state.New(witness.Root(), state.NewDatabase(triedb.NewDatabase(memdb, triedb.HashDefaults), nil))
	if err != nil {
		return common.Hash{}, common.Hash{}, err
	}
	assert.Always(db != nil, "State database should be created successfully", nil)
	if block.NumberU64() > 0 {
		assert.Always(block.ParentHash() != (common.Hash{}), "Non-genesis block must have parent hash", nil)
	}
	chain := &HeaderChain{
		config:      config,
		chainDb:     memdb,
		headerCache: lru.NewCache[common.Hash, *types.Header](256),
		engine:      beacon.New(ethash.NewFaker()),
	}
	assert.Always(chain != nil, "HeaderChain should be created successfully", nil)
	processor := NewStateProcessor(config, chain)
	assert.Always(processor != nil, "StateProcessor should be created successfully", nil)
	validator := NewBlockValidator(config, nil)
	assert.Always(validator != nil, "BlockValidator should be created successfully", nil)
	res, err := processor.Process(block, db, vm.Config{})
	if err != nil {
		return common.Hash{}, common.Hash{}, err
	}
	assert.Always(res != nil, "Processor result should not be nil", nil)
	if err = validator.ValidateState(block, db, res, true); err != nil {
		return common.Hash{}, common.Hash{}, err
	}
	receiptRoot := types.DeriveSha(res.Receipts, trie.NewStackTrie(nil))
	assert.Always(receiptRoot != (common.Hash{}), "Receipt root should not be empty", nil)
	stateRoot := db.IntermediateRoot(config.IsEIP158(block.Number()))
	assert.Always(stateRoot != (common.Hash{}), "State root should not be empty", nil)
	return stateRoot, receiptRoot, nil
}