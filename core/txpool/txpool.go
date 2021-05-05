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
	"errors"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// txSlotSize is used to calculate how many data slots a single transaction
	// takes up based on its size. The slots are used as DoS protection, ensuring
	// that validating a new transaction remains a constant operation (in reality
	// O(maxslots), where max slots are 4 currently).
	txSlotSize = 32 * 1024
	// txMaxSize is the maximum size a single transaction can have. This field has
	// non-trivial consequences: larger transactions are significantly harder and
	// more expensive to propagate; larger transactions also take more resources
	// to validate whether they fit into the pool or not.
	txMaxSize = 4 * txSlotSize // 128KB
)

var (
	// ErrAlreadyKnown is returned if the transactions is already contained
	// within the pool.
	ErrAlreadyKnown = errors.New("already known")

	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrInvalidSender = errors.New("invalid sender")

	// ErrUnderpriced is returned if a transaction's gas price is below the minimum
	// configured for the transaction pool.
	ErrUnderpriced = errors.New("transaction underpriced")

	// ErrTxPoolOverflow is returned if the transaction pool is full and can't accpet
	// another remote transaction.
	ErrTxPoolOverflow = errors.New("txpool is full")

	// ErrReplaceUnderpriced is returned if a transaction is attempted to be replaced
	// with a different one without the required price bump.
	ErrReplaceUnderpriced = errors.New("replacement transaction underpriced")

	// ErrGasLimit is returned if a transaction's requested gas limit exceeds the
	// maximum allowance of the current block.
	ErrGasLimit = errors.New("exceeds block gas limit")

	// ErrNegativeValue is a sanity error to ensure no one is able to specify a
	// transaction with a negative value.
	ErrNegativeValue = errors.New("negative value")

	// ErrOversizedData is returned if the input data of a transaction is greater
	// than some meaningful limit a user might use. This is not a consensus error
	// making the transaction invalid, rather a DOS protection.
	ErrOversizedData = errors.New("oversized data")
)

// blockChain provides the state of blockchain and current gas limit to do
// some pre checks in tx pool and event subscribers.
type blockChain interface {
	CurrentBlock() *types.Block
	GetBlock(hash common.Hash, number uint64) *types.Block
	StateAt(root common.Hash) (*state.StateDB, error)

	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription
}

var _ core.TxPoolIf = (*TxPool)(nil)

type TxPoolConfig struct {
	istanbul bool // Fork indicator whether we are in the istanbul stage.
	eip2718  bool // Fork indicator whether we are using EIP-2718 type transactions.

	PriceBump  int
	MaxTxCount int
	NoLocals   bool

	// maximal gas in block
	maxGasPerBlock uint64
	// minimal gas price
	minGasPrice *big.Int
	// pendingBlockSize determines how many transactions should be returned to
	// the miner when it requests the best transactions from the pool.
	// it is computed as the max gas per block / 21.000
	pendingBlockSize uint64
}

type TxPool struct {
	// Collect data from the chain
	chainconfig   *params.ChainConfig
	chain         blockChain
	currentState  *state.StateDB // Current state in the blockchain head
	pendingNonces *txNoncer      // Pending state tracking virtual nonces

	config *TxPoolConfig
	// feed for notifying about new tx
	txFeed event.Feed
	scope  event.SubscriptionScope
	// subscription for new head events
	chainHeadSub event.Subscription
	// all transactions
	all          *txLookup
	localSenders *senderSet
	signer       types.Signer
	localTxs     *txList
	remoteTxs    *txList
	// global txpool mutex
	mu *sync.RWMutex
}

// Stop stops the transaction pool, closes all registered subscriptions,
// unsubscribes from the blockchain, write all pending transactions to disk.
func (pool *TxPool) Stop() {
	// Unsubscribe all subscriptions registered from txpool
	pool.scope.Close()
	// Unsubscribe subscriptions registered from blockchain
	pool.chainHeadSub.Unsubscribe()
	// TODO: wait for the main loop to shutdown
	// TODO: Write all missing transactions to disk
}

// SubscribeNewTxsEvent registers a subscription of NewTxsEvent and
// starts sending event to the given channel.
func (pool *TxPool) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

// SetGasPrice updates the minimum price required by the transaction pool for a
// new transaction, and drops all transactions below this threshold.
func (pool *TxPool) SetGasPrice(price *big.Int) { panic("not implemented") }

// Nonce returns the next nonce of an account, with all transactions executable
// by the pool already applied on top. This is the pending nonce of the account.
func (pool *TxPool) Nonce(addr common.Address) uint64 {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.pendingNonces.get(addr)
}

// Stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (pool *TxPool) Stats() (int, int) { panic("not implemented") }

// Content retrieves the data content of the transaction pool, returning all the
// pending as well as queued transactions, grouped by account and sorted by nonce.
func (pool *TxPool) Content() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	panic("not implemented")
}

// Pending retrieves all currently processable transactions, grouped by origin
// account and sorted by nonce.
func (pool *TxPool) Pending() (map[common.Address]types.Transactions, error) {
	panic("not implemented")
}

// PendingBlock retrieves the best currently available and executable transactions.
// The PendingTransactions are in two classes: local and remote transactions.
func (pool *TxPool) PendingBlock() (locals types.Transactions, remotes types.Transactions) {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	locals = pool.localTxs.Peek(int(pool.config.pendingBlockSize))
	missing := int(pool.config.pendingBlockSize) - len(locals)
	if missing > 0 {
		remotes = pool.remoteTxs.Peek(missing)
		missing -= len(remotes)
	}
	if missing > 0 {
		panic("TODO lookup tx's from disk if we don't have enough in ram")
	}
	return
}

// Locals retrieves the accounts currently considered local by the pool.
func (pool *TxPool) Locals() []common.Address {
	// TODO: this method is not needed anymore, once the miner is cleaned up
	accounts := make([]common.Address, 0, len(pool.localSenders.accounts))
	for account := range pool.localSenders.accounts {
		accounts = append(accounts, account)
	}
	return accounts
}

// AddLocal enqueues a single local transaction into the pool if it is valid.
// It marks the sending account as local, meaning all further transactions are considered local.
func (pool *TxPool) AddLocal(tx *types.Transaction) error {
	errs := pool.addTxs([]*types.Transaction{tx}, !pool.config.NoLocals, true)
	return errs[0]
}

// AddRemotes enqueues a batch of transactions into the pool if they are valid. If the
// senders are not among the locally tracked ones, full pricing constraints will apply.
//
// This method is used to add transactions from the p2p network and does not wait for pool
// reorganization and internal event propagation.
func (pool *TxPool) AddRemotes(txs []*types.Transaction) []error {
	return pool.addTxs(txs, false, false)
}

// This is like AddRemotes, but waits for pool reorganization. Tests use this method.
func (pool *TxPool) AddRemotesSync(txs []*types.Transaction) []error {
	return pool.addTxs(txs, false, true)
}

// Get returns a transaction if it is contained in the pool and nil otherwise.
func (pool *TxPool) Get(hash common.Hash) *types.Transaction {
	return pool.all.Get(hash)
}

// Has returns true if a transaction is contained in the pool.
func (pool *TxPool) Has(hash common.Hash) bool {
	return pool.all.Has(hash)
}

func (pool *TxPool) addTxs(txs []*types.Transaction, local, sync bool) []error {
	// Filter out known ones without obtaining the pool lock or recovering signatures
	var (
		errs = make([]error, len(txs))
		news = make([]*txEntry, 0, len(txs))
	)
	for i, tx := range txs {
		// If the transaction is known, pre-set the error slot
		if pool.all.Has(tx.Hash()) {
			errs[i] = ErrAlreadyKnown
			continue
		}
		// Exclude transactions with invalid signatures as soon as
		// possible and cache senders in transactions before
		// obtaining lock
		sender, err := types.Sender(pool.signer, tx)
		if err != nil {
			errs[i] = ErrInvalidSender
			continue
		}
		// Accumulate all unknown transactions for deeper processing
		news = append(news, &txEntry{tx: tx, sender: sender, price: tx.GasPrice()})
	}
	if len(news) == 0 {
		return errs
	}
	// Process all the new transaction and merge any errors into the original slice
	pool.mu.Lock()
	newErrs := pool.addTxsLocked(news, local)
	pool.mu.Unlock()

	var nilSlot = 0
	for _, err := range newErrs {
		for errs[nilSlot] != nil {
			nilSlot++
		}
		errs[nilSlot] = err
		nilSlot++
	}
	return errs
}

// addTxsLocked attempts to queue a batch of transactions if they are valid.
// The transaction pool lock must be held.
func (pool *TxPool) addTxsLocked(txs []*txEntry, local bool) []error {
	errs := make([]error, len(txs))
	for i, tx := range txs {
		_, err := pool.add(tx, local)
		errs[i] = err
	}
	return errs
}

// add validates a transaction and inserts it into the non-executable queue for later
// pending promotion and execution. If the transaction is a replacement for an already
// pending or queued one, it overwrites the previous transaction if its price is higher.
//
// If a newly added transaction is marked as local, its sending account will be
// whitelisted, preventing any associated transaction from being dropped out of the pool
// due to pricing constraints.
func (pool *TxPool) add(tx *txEntry, local bool) (bool, error) {
	// If the transaction is already known, discard it
	hash := tx.tx.Hash()
	if pool.all.Has(hash) {
		log.Trace("Discarding already known transaction", "hash", hash)
		return false, ErrAlreadyKnown
	}
	// Make the local flag. If it's from local source or it's from the network but
	// the sender is marked as local previously, treat it as the local transaction.
	isLocal := local || pool.localSenders.contains(tx.sender)
	isReplacement := pool.pendingNonces.get(tx.sender) < tx.tx.Nonce()
	isGapped := pool.pendingNonces.get(tx.sender)+1 < tx.tx.Nonce()
	// If the transaction fails basic validation, discard it
	if err := pool.validateTx(tx.tx, isLocal); err != nil {
		log.Trace("Discarding invalid transaction", "hash", hash, "err", err)
		return false, err
	}

	// If the sender was not in the local senders,
	// we need to add all transactions of this sender to the local txs
	// even if the tx is not valid in the end.
	if !pool.localSenders.contains(tx.sender) {
		defer func() {
			for {
				if entry := pool.remoteTxs.Delete(func(e *txEntry) bool {
					return e.sender == tx.sender
				}); entry != nil {
					pool.localTxs.Add(entry)
					pool.all.Remove(entry.tx.Hash())
					pool.all.Add(entry.tx, true)
				} else {
					break
				}
			}
		}()
	}

	// If this is a replacement transaction, we have to replace it
	if isReplacement {
		return pool.addReplacementTx(tx, isLocal)
	}
	// If it is a queued transaction, we can write it to disk
	if isGapped {
		return false, pool.addGapped(tx, isLocal)
	}
	// If it is the transaction with pendingNonce + 1,
	// we need to insert it and maybe add some now valid transactions
	// from the queued list into the pool
	return false, pool.addContinuousTx(tx, isLocal)
}

func (pool *TxPool) addReplacementTx(tx *txEntry, isLocal bool) (bool, error) {
	// If the transaction is local, insert it into the local pool
	if isLocal {
		replaced := false
		entry := pool.localTxs.Delete(func(e *txEntry) bool {
			return e.sender == tx.sender && e.tx.Nonce() == tx.tx.Nonce()
		})
		if entry != nil {
			if ableToReplace(tx, entry, pool.config.PriceBump) {
				log.Info("Replacing tx: %v with %v", entry.tx.Hash(), tx.tx.Hash())
				pool.all.Remove(entry.tx.Hash())
				replaced = true
			} else {
				// Re-add the deleted tx to the pool
				pool.localTxs.Add(entry)
				log.Trace("Discarding underpriced transaction", "hash", tx.tx.Hash(), "price", tx.tx.GasPrice())
				return false, ErrUnderpriced
			}
		}
		pool.localTxs.Add(tx)
		pool.all.Add(tx.tx, true)

		return replaced, nil
	}
	// If the tx pays less than what we have in memory
	// we can directly replace it on disk.
	if pool.remoteTxs.Len() > pool.config.MaxTxCount && tx.Less(pool.remoteTxs.LastEntry()) {
		// TODO write directly to disk
	}
	replaced := false
	entry := pool.remoteTxs.Delete(func(e *txEntry) bool {
		return e.sender == tx.sender && e.tx.Nonce() == tx.tx.Nonce()
	})
	if entry != nil {
		if ableToReplace(tx, entry, pool.config.PriceBump) {
			log.Info("Replacing tx: %v with %v", entry.tx.Hash(), tx.tx.Hash())
			pool.all.Remove(entry.tx.Hash())
			replaced = true
		} else {
			// Re-add the deleted tx to the pool
			pool.remoteTxs.Add(entry)
			log.Trace("Discarding underpriced transaction", "hash", tx.tx.Hash(), "price", tx.tx.GasPrice())
			return false, ErrUnderpriced
		}
	}
	shouldPrune := pool.remoteTxs.Add(tx)
	pool.all.Add(tx.tx, false)
	if shouldPrune {
		// TODO prune in memory list to disk
	}
	return replaced, nil
}

func (pool *TxPool) addGapped(tx *txEntry, local bool) error {
	panic("not implemented")
}

func (pool *TxPool) addContinuousTx(tx *txEntry, local bool) error {
	panic("not implemented")
}

// validateTx checks whether a transaction is valid according to the consensus
// rules and adheres to some heuristic limits of the local node (price and size).
func (pool *TxPool) validateTx(tx *types.Transaction, local bool) error {
	// Accept only legacy transactions until EIP-2718/2930 activates.
	if !pool.config.eip2718 && tx.Type() != types.LegacyTxType {
		return core.ErrTxTypeNotSupported
	}
	// Reject transactions over defined size to prevent DOS attacks
	if uint64(tx.Size()) > txMaxSize {
		return ErrOversizedData
	}
	// Transactions can't be negative. This may never happen using RLP decoded
	// transactions but may occur if you create a transaction using the RPC.
	if tx.Value().Sign() < 0 {
		return ErrNegativeValue
	}
	// Ensure the transaction doesn't exceed the current block limit gas.
	if pool.config.maxGasPerBlock < tx.Gas() {
		return ErrGasLimit
	}
	// Make sure the transaction is signed properly.
	from, err := types.Sender(pool.signer, tx)
	if err != nil {
		return ErrInvalidSender
	}
	// Drop non-local transactions under our own minimal accepted gas price
	if !local && tx.GasPriceIntCmp(pool.config.minGasPrice) < 0 {
		return ErrUnderpriced
	}
	// Ensure the transaction adheres to nonce ordering
	if pool.currentState.GetNonce(from) > tx.Nonce() {
		return core.ErrNonceTooLow
	}
	// Transactor should have enough funds to cover the costs
	// cost == V + GP * GL
	if pool.currentState.GetBalance(from).Cmp(tx.Cost()) < 0 {
		return core.ErrInsufficientFunds
	}
	// Ensure the transaction has more gas than the basic tx fee.
	intrGas, err := core.IntrinsicGas(tx.Data(), tx.AccessList(), tx.To() == nil, true, pool.config.istanbul)
	if err != nil {
		return err
	}
	if tx.Gas() < intrGas {
		return core.ErrIntrinsicGas
	}
	return nil
}

func ableToReplace(new, old *txEntry, priceBump int) bool {
	// threshold = oldGP * (100 + priceBump) / 100
	a := big.NewInt(100 + int64(priceBump))
	a = a.Mul(a, old.price)
	b := big.NewInt(100)
	threshold := a.Div(a, b)
	// Have to ensure that the new gas price is higher than the old gas
	// price as well as checking the percentage threshold to ensure that
	// this is accurate for low (Wei-level) gas price replacements
	if old.price.Cmp(new.price) < 0 && new.price.Cmp(threshold) >= 0 {
		return true
	}
	return false
}
