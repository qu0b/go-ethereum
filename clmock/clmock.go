package clmock

import (
	"context"
	"time"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

type CLMock struct {
	ctx context.Context
	cancel context.CancelFunc
}

func (c *CLMock) Start() {
	c.ctx, c.cancel = context.WithCancel(context.Background())
	go c.clmockLoop()
}

func (c * CLMock) Stop() {
	c.ctx.Done()
	c.cancel()
}

// TODO: use ctx with timeout when calling rpc methods? is there a way they could hang indefinitely (even though we are calling on same machine/process)?

func (c *CLMock) clmockLoop() {
	ticker := time.NewTicker(time.Millisecond * 500)
	blockPeriod := time.Second * 2
	lastBlockTime := time.Now()

	var curForkchoiceState *engine.ForkchoiceStateV1
	var prevRandaoVal common.Hash
	var suggestedFeeRecipient common.Address

	engine_api := engineAPI{}
	if err := engine_api.Connect(c.ctx, "http://127.0.0.1:8545"); err != nil {
		log.Error("failed to connect to engine api: %v", err)
	}

	header, err := engine_api.GetHeaderByNumber(c.ctx, 0)
	if err != nil {
		log.Error("failed to get genesis block header", err)
	}

	curForkchoiceState = &engine.ForkchoiceStateV1{
		HeadBlockHash: header.Hash(),
		SafeBlockHash: header.Hash(),
		FinalizedBlockHash: header.Hash(),
	}

	_, err = engine_api.ForkchoiceUpdatedV1(c.ctx, curForkchoiceState, nil)

	if err != nil {
		log.Error("failed to initiate PoS transition for genesis via Forkchoiceupdated", err)
	}

	for {
		select {
		case _ = <-c.ctx.Done():
			break
		case curTime := <-ticker.C:
			if curTime.After(lastBlockTime.Add(blockPeriod)) {
				// get the current head and populate curForkchoiceState

				safeHead, err := engine_api.GetHeaderByTag(c.ctx, "safe")
				if err != nil {
					log.Error("failed to get safe header", err)
				}
				finalizedHead, err := engine_api.GetHeaderByTag(c.ctx, "finalized")
				if err != nil {
					log.Error("failed to get finalized header", err)
				}

				// send forkchoiceupdated (to trigger block building)
				fcState, err := engine_api.ForkchoiceUpdatedV1(c.ctx, curForkchoiceState, &engine.PayloadAttributes{
					Timestamp: uint64(curTime.Unix()), // TODO make sure conversion from int64->uint64 is okay here (should be fine)
					Random: prevRandaoVal,
					SuggestedFeeRecipient: suggestedFeeRecipient,
				})

				if err != nil {
					log.Error("failed to trigger block building via forkchoiceupdated", err)
				}

				var payload *engine.ExecutableData

				buildTicker := time.NewTicker(50 * time.Millisecond)
				// spin a bit until the payload is built
				for {
					var done bool
					select {
					case _ = <-buildTicker.C:
						// try and get the payload
						payload, err = engine_api.GetPayloadV1(c.ctx, fcState.PayloadID)
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
				if err = engine_api.NewPayloadV1(c.ctx, payload); err != nil {
					log.Error("failed to mark payload as canonical: %v", err)
				}

				newForkchoiceState := &engine.ForkchoiceStateV1{
					HeadBlockHash: payload.BlockHash,
					SafeBlockHash: safeHead.Hash(),
					FinalizedBlockHash: finalizedHead.Hash(),
				}

				// send Forkchoiceupdated (TODO: only if the payload had transactions)
				_, err = engine_api.ForkchoiceUpdatedV1(c.ctx, newForkchoiceState, nil)
				if err != nil {
					log.Error("failed to mark block as canonical", err)
				}
				lastBlockTime = time.Now()
				curForkchoiceState = newForkchoiceState
			}
		}
	}
}
