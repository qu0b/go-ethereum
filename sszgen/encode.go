package main

import (
	"encoding/binary"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

func EncodeBasic(v any) []byte {
	switch v := v.(type) {
	case uint32:
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, v)
		return b
	case uint64:
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, v)
		return b
	case [32]byte:
		return v[:]
	case common.Hash:
		return v[:]
	case *big.Int:
		b := make([]byte, 32)
		copy(b, v.Bytes())
		return b
	}
	return []byte{}
}
