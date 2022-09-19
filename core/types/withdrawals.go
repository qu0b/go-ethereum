package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type Withdrawal struct {
	Index     uint64
	Recipient common.Address
	Amount    *big.Int
}
