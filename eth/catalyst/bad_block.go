package catalyst

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	txfuzz "github.com/MariusVanDerWijden/tx-fuzz"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/beacon"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/trie"
)

func weirdHash(data *beacon.ExecutableData, hashes ...common.Hash) common.Hash {
	rnd := rand.Int()
	switch rnd % 10 {
	case 0:
		return common.Hash{}
	case 1:
		return data.BlockHash
	case 2:
		return data.ParentHash
	case 3:
		return data.StateRoot
	case 4:
		return data.ReceiptsRoot
	case 5:
		return data.Random
	case 6:
		return hashes[rand.Int31n(int32(len(hashes)))]
	default:
		hash := hashes[rand.Int31n(int32(len(hashes)))]
		newBytes := hash.Bytes()
		index := rand.Int31n(int32(len(newBytes)))
		i := rand.Int31n(8)
		newBytes[index] = newBytes[index] ^ 1<<i
		return common.BytesToHash(newBytes)
	}
}

func weirdNumber(data *beacon.ExecutableData, number uint64) uint64 {
	rnd := rand.Int()
	switch rnd % 7 {
	case 0:
		return 0
	case 1:
		return 1
	case 2:
		return rand.Uint64()
	case 3:
		return ^uint64(0)
	case 4:
		return number + 1
	case 5:
		return number - 1
	default:
		return number + uint64(rand.Int63n(100000))
	}
}

func weirdByteSlice(data []byte) []byte {
	rnd := rand.Int()
	switch rnd % 4 {
	case 0:
		return make([]byte, 0)
	case 1:
		return make([]byte, 257)
	case 2:
		return []byte{1, 2}
	case 3:
		slice := make([]byte, len(data))
		rand.Read(slice)
		return slice
	default:
		return data
	}
}

func (api *ConsensusAPI) mutateExecutableData(data *beacon.ExecutableData) *beacon.ExecutableData {
	hashes := []common.Hash{
		data.BlockHash,
		data.ParentHash,
		api.eth.BlockChain().GetCanonicalHash(0),
		api.eth.BlockChain().GetCanonicalHash(data.Number - 255),
		api.eth.BlockChain().GetCanonicalHash(data.Number - 256),
		api.eth.BlockChain().GetCanonicalHash(data.Number - 257),
		api.eth.BlockChain().GetCanonicalHash(data.Number - 1000),
		api.eth.BlockChain().GetCanonicalHash(data.Number - 90001),
	}
	bloom := types.BytesToBloom(data.LogsBloom)
	rnd := rand.Int()
	switch rnd % 60 {
	case 1:
		data.BlockHash = weirdHash(data, hashes...)
	case 2:
		data.ParentHash = weirdHash(data, hashes...)
	case 3:
		data.FeeRecipient = common.Address{}
	case 4:
		data.StateRoot = weirdHash(data, data.StateRoot)
	case 5:
		data.ReceiptsRoot = weirdHash(data, data.ReceiptsRoot)
	case 6:
		bloom.SetBytes(weirdByteSlice(data.LogsBloom))
	case 7:
		data.Random = weirdHash(data, data.Random)
	case 8:
		data.Number = weirdNumber(data, data.Number)
	case 9:
		data.GasLimit = weirdNumber(data, data.GasLimit)
	case 10:
		data.GasUsed = weirdNumber(data, data.GasUsed)
	case 11:
		data.Timestamp = weirdNumber(data, data.Timestamp)
	case 12:
		hash := weirdHash(data, common.Hash{})
		data.ExtraData = hash[:]
	case 13:
		data.BaseFeePerGas = big.NewInt(int64(weirdNumber(data, data.BaseFeePerGas.Uint64())))
	case 14:
		data.BlockHash = weirdHash(data, data.BlockHash)
	}
	if rand.Int()%1 == 0 {
		// Set correct blockhash in 50% of cases
		txs, _ := decodeTx(data.Transactions)
		txs, txhash := api.mutateTransactions(txs)
		number := big.NewInt(0)
		number.SetUint64(data.Number)
		withdrawals, withdrawalHash := api.mutateWithdrawals(data.Withdrawals)
		header := &types.Header{
			ParentHash:      data.ParentHash,
			UncleHash:       types.EmptyUncleHash,
			Coinbase:        data.FeeRecipient,
			Root:            data.StateRoot,
			TxHash:          txhash,
			ReceiptHash:     data.ReceiptsRoot,
			Bloom:           bloom,
			Difficulty:      common.Big0,
			Number:          number,
			GasLimit:        data.GasLimit,
			GasUsed:         data.GasUsed,
			Time:            data.Timestamp,
			BaseFee:         data.BaseFeePerGas,
			Extra:           data.ExtraData,
			MixDigest:       data.Random,
			WithdrawalsHash: withdrawalHash,
		}
		block := types.NewBlockWithHeader(header).WithBody(txs, nil /* uncles */).WithWithdrawals(withdrawals)
		data.BlockHash = block.Hash()
	}
	return data
}
func decodeTx(enc [][]byte) ([]*types.Transaction, error) {
	var txs = make([]*types.Transaction, len(enc))
	for i, encTx := range enc {
		var tx types.Transaction
		if err := tx.UnmarshalBinary(encTx); err != nil {
			return nil, fmt.Errorf("invalid transaction %d: %v", i, err)
		}
		txs[i] = &tx
	}
	return txs, nil
}

func (api *ConsensusAPI) mutateWithdrawals(withdrawals []*types.Withdrawal) ([]*types.Withdrawal, *common.Hash) {
	var withdrawalHash *common.Hash
	w := types.DeriveSha(types.Withdrawals(withdrawals), trie.NewStackTrie(nil))
	withdrawalHash = &w
	rnd := rand.Int()
	switch rnd % 10 {
	case 1:
		// duplicate a withdrawal
		w := withdrawals[rand.Intn(len(withdrawals))]
		withdrawals = append(withdrawals, w)
	case 2:
		// replace a withdrawal
		index := rand.Intn(len(withdrawals))
		b := make([]byte, 32)
		rand.Read(b)
		w := types.Withdrawal{
			Index:     rand.Uint64(),
			Validator: rand.Uint64(),
			Address:   common.BytesToAddress(b),
			Amount:    rand.Uint64(),
		}
		withdrawals[index] = &w
	case 3:
		// modify a withdrawal
		w := withdrawals[rand.Intn(len(withdrawals))]
		field := rand.Int()
		switch field % 4 {
		case 0:
			w.Index = rand.Uint64()
		case 1:
			w.Validator = rand.Uint64()
		case 2:
			w.Amount = rand.Uint64()
		case 3:
			b := make([]byte, 32)
			rand.Read(b)
			w.Address = common.BytesToAddress(b)
		}
	}

	if rand.Int()%100 < 70 {
		// Recompute correct txhash in most cases
		w = types.DeriveSha(types.Withdrawals(withdrawals), trie.NewStackTrie(nil))
		withdrawalHash = &w
	} else {
		switch rand.Int() % 5 {
		case 0:
			withdrawalHash = nil
		case 1:
			withdrawalHash = &types.EmptyRootHash
		case 2:
			withdrawalHash = &types.EmptyUncleHash
		case 3:
			withdrawalHash = &common.Hash{}
		case 4:
			b := make([]byte, 32)
			rand.Read(b)
			w := common.BytesToHash(b)
			withdrawalHash = &w
		}
	}
	return withdrawals, withdrawalHash
}

func (api *ConsensusAPI) mutateTransactions(txs []*types.Transaction) ([]*types.Transaction, common.Hash) {
	txhash := types.DeriveSha(types.Transactions(txs), trie.NewStackTrie(nil))
	rnd := rand.Int()
	add := 0
	// if no txs are available, don't duplicate/modify any
	if len(txs) == 0 {
		add = 3
	}
	switch rnd%20 + add {
	case 1:
		// duplicate a txs
		tx := txs[rand.Intn(len(txs))]
		txs = append(txs, tx)
	case 2:
		// replace a tx
		index := rand.Intn(len(txs))
		b := make([]byte, 200)
		rand.Read(b)
		tx, err := txfuzz.RandomTx(filler.NewFiller(b))
		if err != nil {
			fmt.Println(err)
		}
		if rand.Int()%2 == 0 {
			txs[index] = tx
		} else {
			key := "0xaf5ead4413ff4b78bc94191a2926ae9ccbec86ce099d65aaf469e9eb1a0fa87f"
			sk := crypto.ToECDSAUnsafe(common.FromHex(key))
			chainid := big.NewInt(0x146998)
			signedTx, err := types.SignTx(tx, types.NewLondonSigner(chainid), sk)
			if err != nil {
				panic(err)
			}
			txs[index] = signedTx
		}
	case 3:
		// Add a huuuge transaction
		gasLimit := uint64(7_800_000)
		code := []byte{0x60, 0x00, 0x60, 0x00, 0x60, 0x00, 0xf3}
		bigSlice := make([]byte, randomSize())
		code = append(code, bigSlice...)
		nonce, err := api.eth.APIBackend.GetPoolNonce(context.Background(), common.HexToAddress("0xb02A2EdA1b317FBd16760128836B0Ac59B560e9D"))
		if err != nil {
			panic(err)
		}
		gasPrice, err := api.eth.APIBackend.SuggestGasTipCap(context.Background())
		if err != nil {
			panic(err)
		}
		tx := types.NewContractCreation(nonce, big.NewInt(0), gasLimit, gasPrice, code)

		key := "0xcdfbe6f7602f67a97602e3e9fc24cde1cdffa88acd47745c0b84c5ff55891e1b"
		sk := crypto.ToECDSAUnsafe(common.FromHex(key))
		chainid := big.NewInt(0x146998)
		signedTx, err := types.SignTx(tx, types.NewLondonSigner(chainid), sk)
		if err != nil {
			panic(err)
		}
		txs = append(txs, signedTx)
	case 4:
		// add lots and lots of transactions
		rounds := rand.Int31n(1000)
		for i := 0; i < int(rounds); i++ {
			b := make([]byte, 200)
			rand.Read(b)
			tx, err := txfuzz.RandomTx(filler.NewFiller(b))
			if err != nil {
				fmt.Println(err)
			}

			key := "0xaf5ead4413ff4b78bc94191a2926ae9ccbec86ce099d65aaf469e9eb1a0fa87f"
			sk := crypto.ToECDSAUnsafe(common.FromHex(key))
			chainid := big.NewInt(0x146998)
			signedTx, err := types.SignTx(tx, types.NewLondonSigner(chainid), sk)
			if err != nil {
				panic(err)
			}
			txs = append(txs, signedTx)
		}
	}

	if rand.Int()%100 < 80 {
		// Recompute correct txhash in most cases
		txhash = types.DeriveSha(types.Transactions(txs), trie.NewStackTrie(nil))
	}
	return txs, txhash
}

func randomSize() int {
	rnd := rand.Int31n(100)
	if rnd < 5 {
		return int(rand.Int31n(11 * 1024 * 1024))
	} else if rnd < 10 {
		return 128*1024 + 1
	} else if rnd < 20 {
		return int(rand.Int31n(128 * 1024))
	}
	return int(rand.Int31n(127 * 1024))
}
