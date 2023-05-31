// Copyright 2023 The go-ethereum Authors
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

package clmock

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/eth/catalyst"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/internal/ethapi"
)

type CLMock struct {
	ctx    context.Context
	cancel context.CancelFunc
	stack  *node.Node
	backend ethapi.Backend
}

func NewCLMock(stack *node.Node, backend ethapi.Backend) *CLMock {
	c := CLMock{}
	c.stack = stack
	c.backend = backend
	return &c
}

// Start invokes the clmock life-cycle function in a goroutine
func (c *CLMock) Start() {
	c.ctx, c.cancel = context.WithCancel(context.Background())
	go c.clmockLoop()
}

// Stop halts the clmock service
func (c *CLMock) Stop() {
	c.cancel()
}

// TODO: use ctx with timeout when calling rpc methods? is there a way they could hang indefinitely (even though we are calling on same machine/process)?

// clmockLoop manages the lifecycle of clmock.
// it drives block production, taking the role of a CL client and interacting with Geth via the engine API
func (c *CLMock) clmockLoop() {
	// TODO: (randomly placed here as a reminder to note it somewhere more prominent:
	// how do we sync node shutdown with this separate go-routine?
	//
	ticker := time.NewTicker(time.Millisecond * 500)
	blockPeriod := time.Second * 2 // hard-coded fast block period for testing purposes
	lastBlockTime := time.Now()

	var curForkchoiceState engine.ForkchoiceStateV1
	var prevRandaoVal common.Hash
	var suggestedFeeRecipient common.Address

	// dangerous? TODO: ensure this first call can't possibly fail
	engs := c.stack.GetAPIsByNamespace("engine")

	eng := engs[0]
	engineAPI, ok := eng.Service.(*catalyst.ConsensusAPI)
	if !ok {
		panic("crap")
	}

	// TODO: don't use APIBackend (access blockchain directly instead)
	header, err := c.backend.HeaderByNumber(context.Background(), 0)
	if err != nil {
		log.Crit("failed to get genesis block header", "err", err)
	}

	curForkchoiceState = engine.ForkchoiceStateV1{
		HeadBlockHash:      header.Hash(),
		SafeBlockHash:      header.Hash(),
		FinalizedBlockHash: header.Hash(),
	}

	_, err = engineAPI.ForkchoiceUpdatedV1(curForkchoiceState, nil)

	if err != nil {
		log.Crit("failed to initiate PoS transition for genesis via Forkchoiceupdated", "err", err)
	}

	for {
		select {
		case <-c.ctx.Done():
			break
		case curTime := <-ticker.C:
			if curTime.After(lastBlockTime.Add(blockPeriod)) {
				// trigger block building (via forkchoiceupdated)
				fcState, err := engineAPI.ForkchoiceUpdatedV1(curForkchoiceState, &engine.PayloadAttributes{
					Timestamp:             uint64(curTime.Unix()), // TODO make sure conversion from int64->uint64 is okay here (should be fine)
					Random:                prevRandaoVal,
					SuggestedFeeRecipient: suggestedFeeRecipient,
				})

				if err != nil {
					log.Crit("failed to trigger block building via forkchoiceupdated", "err", err)
				}

				var payload *engine.ExecutableData

				buildTicker := time.NewTicker(50 * time.Millisecond)
				// spin a bit until the payload is built
				for {
					var done bool
					select {
					case _ = <-buildTicker.C:
						// try and get the payload
						payload, err = engineAPI.GetPayloadV1(*fcState.PayloadID)
						if err != nil {
							// TODO: if err is that the payload is still building, continue spinning
							// otherwise: fail hard (?)
							panic(err)
						}
						done = true
						break
					case _ = <-c.ctx.Done():
						return
					}
					if done {
						break
					}
				}

				if len(payload.Transactions) == 0 {
					// don't create a block if there are no transactions
					log.Warn("no transactions.  waiting for more")
					continue
				}

				// mark the payload as canon
				if _, err = engineAPI.NewPayloadV1(*payload); err != nil {
					log.Crit("failed to mark payload as canonical", "err", err)
				}

				newForkchoiceState := &engine.ForkchoiceStateV1{
					HeadBlockHash:      payload.BlockHash,
					SafeBlockHash:      payload.BlockHash,
					FinalizedBlockHash: payload.BlockHash,
				}

				// send Forkchoiceupdated (TODO: only if the payload had transactions)
				_, err = engineAPI.ForkchoiceUpdatedV1(*newForkchoiceState, nil)
				if err != nil {
					log.Crit("failed to mark block as canonical", "err", err)
				}
				lastBlockTime = time.Now()
				curForkchoiceState = *newForkchoiceState
			}
		}
	}
}
