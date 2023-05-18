package clmock

import (
	"context"
	"time"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
)

type CLMock struct {
	ctx context.Context
}

func (c *CLMock) Start() {
	go c.clmockLoop()
}

func (c *CLMock) clmockLoop() {
	ticker := time.NewTicker(time.Millisecond * 500)
	blockPeriod := time.Second * 12
	lastBlockTime := time.Now()

	var curForkchoiceState *engine.ForkchoiceStateV1
	var prevRandaoVal common.Hash
	var suggestedFeeRecipient common.Address

	engine_api := engineAPI{}
	if err := engine_api.Connect(c.ctx, "http://127.0.0.1:8545"); err != nil {
		panic(err)
	}

	header, err := engine_api.GetHeaderByNumber(0)
	if err != nil {
		panic(err)
	}

	_, err = engine_api.ForkchoiceUpdatedV1(&engine.ForkchoiceStateV1{
		HeadBlockHash: header.Hash(),
		SafeBlockHash: header.Hash(),
		FinalizedBlockHash: header.Hash(),
	}, nil)

	if err != nil {
		panic(err)
	}

	for {
		select {
		case _ = <-c.ctx.Done():
			break
		case curTime := <-ticker.C:
			if curTime.After(lastBlockTime.Add(blockPeriod)) {
				// get the current head and populate curForkchoiceState

				// send forkchoiceupdated (to trigger block building)
				fcState, err := engine_api.ForkchoiceUpdatedV1(curForkchoiceState, &engine.PayloadAttributes{
					Timestamp: uint64(curTime.Unix()), // TODO make sure conversion from int64->uint64 is okay here (should be fine)
					Random: prevRandaoVal,
					SuggestedFeeRecipient: suggestedFeeRecipient,
				})

				if err != nil {
					// TODO: log error and hard-quit
					panic(err)
				}

				var payload *engine.ExecutableData

				buildTicker := time.NewTicker(100 * time.Millisecond)
				// spin a bit until the payload is built
				for {
					select {
					case _ = <-buildTicker.C:
						// try and get the payload
						payload, err = engine_api.GetPayloadV1(fcState.PayloadID)
						if err != nil {
							// TODO: if err is that the payload is still building, continue spinning
							panic(err)
						}
					case _ := <-c.ctx.Done():
						return
					}
				}

/*
				// short-circuit if the payload didn't have transactions
				if len(payload.Transactions) == 0 {
					// TODO: more intelligent handling here
					continue
				}
*/

				// mark the payload as canon
				if err = engine_api.NewPayloadV1(payload); err != nil {
					panic(err)
				}

				// send Forkchoiceupdated (if the payload had transactions)
				_, err = engine_api.ForkChoiceUpdatedV1(engine.ForkChoiceStateV1{
					HeadBlockHash: payload.BlockHash,
					SafeBlockHash: fcState.Safe.Hash,
					FinalizedBlockHash: fcState.Finalized.Hash,
				}, nil)
				if err != nil {
					panic(err)
				}
			}
		}
	}
}

/*
func (c *CLMock) Stop() {
	c.ctx.Cancel()
}
*/
