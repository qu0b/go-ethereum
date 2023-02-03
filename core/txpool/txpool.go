package txpool

import (
	"database/sql"
	"fmt"
	"math/big"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10

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

// blockChain provides the state of blockchain and current gas limit to do
// some pre checks in tx pool and event subscribers.
type blockChain interface {
	CurrentBlock() *types.Block
	GetBlock(hash common.Hash, number uint64) *types.Block
	StateAt(root common.Hash) (*state.StateDB, error)

	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription
}

// Config are the configuration parameters of the transaction pool.
type Config struct {
	Locals    []common.Address // Addresses that should be treated by default as local
	NoLocals  bool             // Whether local transaction handling should be disabled
	Journal   string           // Journal of local transactions to survive node restarts
	Rejournal time.Duration    // Time interval to regenerate the local transaction journal

	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)

	AccountSlots uint64 // Number of executable transaction slots guaranteed per account
	GlobalSlots  uint64 // Maximum number of executable transaction slots for all accounts
	AccountQueue uint64 // Maximum number of non-executable transaction slots permitted per account
	GlobalQueue  uint64 // Maximum number of non-executable transaction slots for all accounts

	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}

// DefaultConfig contains the default configurations for the transaction
// pool.
var DefaultConfig = Config{
	Journal:   "transactions.rlp",
	Rejournal: time.Hour,

	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 16,
	GlobalSlots:  4096 + 1024, // urgent + floating queue capacity with 4:1 ratio
	AccountQueue: 64,
	GlobalQueue:  1024,

	Lifetime: 3 * time.Hour,
}

// TxStatus is the current status of a transaction as seen by the pool.
type TxStatus uint

const (
	TxStatusUnknown TxStatus = iota
	TxStatusQueued
	TxStatusPending
	TxStatusIncluded
)

type TxPool struct {
	istanbul bool // Fork indicator whether we are in the istanbul stage.
	eip2718  bool // Fork indicator whether we are using EIP-2718 type transactions.
	eip1559  bool // Fork indicator whether we are using EIP-1559 type transactions.
	shanghai bool // Fork indicator whether we are in the Shanghai stage.

	currentMaxGas uint64
	signer        types.Signer
	gasPrice      *big.Int
	currentState  *state.StateDB // Current state in the blockchain head
	config        Config

	db  *sql.DB
	txs *lookup
}

type tx struct {
	Hash      common.Hash
	Local     bool
	Sender    common.Address
	Nonce     uint64
	Cost      uint64
	GasFeeCap uint64
	Tx        *types.Transaction
}

func NewTxPool(config Config, chainconfig *params.ChainConfig, chain blockChain) *TxPool {
	db, err := initDB()
	if err != nil {
		panic(err)
	}
	return &TxPool{
		db:     db,
		txs:    newLookup(),
		config: config,
	}
}

// validateTx checks whether a transaction is valid according to the consensus
// rules and adheres to some heuristic limits of the local node (price and size).
func (pool *TxPool) validateTx(tx *types.Transaction, local bool) error {
	// Accept only legacy transactions until EIP-2718/2930 activates.
	if !pool.eip2718 && tx.Type() != types.LegacyTxType {
		return core.ErrTxTypeNotSupported
	}
	// Reject dynamic fee transactions until EIP-1559 activates.
	if !pool.eip1559 && tx.Type() == types.DynamicFeeTxType {
		return core.ErrTxTypeNotSupported
	}
	// Reject transactions over defined size to prevent DOS attacks
	if tx.Size() > txMaxSize {
		return ErrOversizedData
	}
	// Check whether the init code size has been exceeded.
	if pool.shanghai && tx.To() == nil && len(tx.Data()) > params.MaxInitCodeSize {
		return fmt.Errorf("%w: code size %v limit %v", core.ErrMaxInitCodeSizeExceeded, len(tx.Data()), params.MaxInitCodeSize)
	}
	// Transactions can't be negative. This may never happen using RLP decoded
	// transactions but may occur if you create a transaction using the RPC.
	if tx.Value().Sign() < 0 {
		return ErrNegativeValue
	}
	// Ensure the transaction doesn't exceed the current block limit gas.
	if pool.currentMaxGas < tx.Gas() {
		return ErrGasLimit
	}
	// Sanity check for extremely large numbers
	if tx.GasFeeCap().BitLen() > 256 {
		return core.ErrFeeCapVeryHigh
	}
	if tx.GasTipCap().BitLen() > 256 {
		return core.ErrTipVeryHigh
	}
	// Ensure gasFeeCap is greater than or equal to gasTipCap.
	if tx.GasFeeCapIntCmp(tx.GasTipCap()) < 0 {
		return core.ErrTipAboveFeeCap
	}
	// Make sure the transaction is signed properly.
	from, err := types.Sender(pool.signer, tx)
	if err != nil {
		return ErrInvalidSender
	}
	// Drop non-local transactions under our own minimal accepted gas price or tip
	if !local && tx.GasTipCapIntCmp(pool.gasPrice) < 0 {
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
	intrGas, err := core.IntrinsicGas(tx.Data(), tx.AccessList(), tx.To() == nil, true, pool.istanbul, pool.shanghai)
	if err != nil {
		return err
	}
	if tx.Gas() < intrGas {
		return core.ErrIntrinsicGas
	}
	return nil
}

func (t *TxPool) AddLocal(tx *types.Transaction) error {
	return t.add([]*types.Transaction{tx}, true)[0]
}

func (t *TxPool) AddLocals(tx types.Transactions) []error {
	return t.add(tx, true)
}

func (t *TxPool) AddRemote(tx *types.Transaction) error {
	return t.add([]*types.Transaction{tx}, false)[0]
}

func (t *TxPool) AddRemotesSync(txs []*types.Transaction) []error {
	return t.add(txs, false)
}

func (t *TxPool) AddRemotes(txs []*types.Transaction) []error {
	return t.add(txs, false)
}

func (t *TxPool) Pending(enforceTips bool) map[common.Address]types.Transactions {
	return nil
}

func (t *TxPool) Locals() []common.Address {
	return nil
}

func (t *TxPool) Get(common.Hash) *types.Transaction {
	return nil
}

func (t *TxPool) Has(common.Hash) bool {
	return false
}

func (t *TxPool) Nonce(addr common.Address) uint64 {
	return 0
}

func (t *TxPool) Stats() (pending int, queued int) {
	return 0, 0
}

func (t *TxPool) Status([]common.Hash) []TxStatus {
	return nil
}

func (t *TxPool) Content() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return nil, nil
}

func (t *TxPool) ContentFrom(addr common.Address) (types.Transactions, types.Transactions) {
	return nil, nil
}

func (t *TxPool) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return nil
}

func (t *TxPool) SetGasPrice(*big.Int) {}

func (t *TxPool) Stop() {}
