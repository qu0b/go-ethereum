package txpool

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type lookup struct {
	slots int
	mu    sync.RWMutex
	txs   map[common.Hash]*types.Transaction
}

func newLookup() *lookup {
	return &lookup{
		txs: make(map[common.Hash]*types.Transaction),
	}
}

func (l *lookup) Add(tx *types.Transaction) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.slots += numSlots(tx)
	l.txs[tx.Hash()] = tx
}

func (l *lookup) Remove(hash common.Hash) *types.Transaction {
	l.mu.Lock()
	defer l.mu.Unlock()

	tx, ok := l.txs[hash]
	if !ok {
		return nil
	}
	l.slots -= numSlots(tx)
	delete(l.txs, hash)
	return tx
}

func (l *lookup) Get(hash common.Hash) *types.Transaction {
	l.mu.RLock()
	defer l.mu.RUnlock()

	tx, ok := l.txs[hash]
	if !ok {
		return nil
	}
	return tx
}

func (l *lookup) Slots() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.slots
}

// numSlots calculates the number of slots needed for a single transaction.
func numSlots(tx *types.Transaction) int {
	return int((tx.Size() + txSlotSize - 1) / txSlotSize)
}
