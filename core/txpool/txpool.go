package txpool

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
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
}

func NewTxPool(config Config, chainconfig *params.ChainConfig, chain blockChain) *TxPool

func (t *TxPool) AddLocal(tx *types.Transaction) error

func (t *TxPool) AddLocals(tx types.Transactions) error

func (t *TxPool) AddRemote(tx *types.Transaction) error

func (t *TxPool) AddRemotesSync(tx types.Transactions) error

func (t *TxPool) AddRemotes(txs []*types.Transaction) []error

func (t *TxPool) Pending(enforceTips bool) map[common.Address]types.Transactions

func (t *TxPool) Locals() []common.Address

func (t *TxPool) Get(common.Hash) *types.Transaction

func (t *TxPool) Has(common.Hash) bool

func (t *TxPool) Nonce(addr common.Address) uint64

func (t *TxPool) Stats() (pending int, queued int)

func (t *TxPool) Content() (map[common.Address]types.Transactions, map[common.Address]types.Transactions)

func (t *TxPool) ContentFrom(addr common.Address) (types.Transactions, types.Transactions)

func (t *TxPool) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription

func (t *TxPool) SetGasPrice(*big.Int)

func (t *TxPool) Stop()
