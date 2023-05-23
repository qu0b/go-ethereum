package clmock

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/rpc"
)

type engineAPI struct {
	client *rpc.Client
}

func (e *engineAPI) Connect(ctx context.Context, endpoint string) error {
	var testSecret = [32]byte{94, 111, 36, 109, 245, 74, 43, 72, 202, 33, 205, 86, 199, 174, 186, 77, 165, 99, 13, 225, 149, 121, 125, 249, 128, 109, 219, 163, 224, 176, 46, 233}
	var testEndpoint = "http://127.0.0.1:8551"

	auth := node.NewJWTAuth(testSecret)
	client, err := rpc.DialOptions(ctx, testEndpoint, rpc.WithHTTPAuth(auth))
	if err != nil {
		return err
	}

	e.client = client
	return nil
}

func (e *engineAPI) ForkchoiceUpdatedV1(ctx context.Context, fcState *engine.ForkchoiceStateV1, payloadAttr *engine.PayloadAttributes) (*engine.ForkChoiceResponse, error) {
	var resp *engine.ForkChoiceResponse
	if err := e.client.CallContext(ctx, &resp, "engine_forkchoiceUpdatedV1", fcState, payloadAttr); err != nil {
		return nil, err
	}
	return resp, nil
}

func (e *engineAPI) GetPayloadV1(ctx context.Context, id *engine.PayloadID) (*engine.ExecutableData, error) {
	var res *engine.ExecutableData
	if err := e.client.CallContext(ctx, &res, "engine_getPayloadV1", id); err != nil {
		return nil, err
	}
	return res, nil
}

func (e *engineAPI) NewPayloadV1(ctx context.Context, payload *engine.ExecutableData) error {
	var res *engine.PayloadStatusV1
	if err := e.client.CallContext(ctx, &res, "engine_newPayloadV1", payload); err != nil {
		return err
	}
	return nil
}

func (e *engineAPI) GetHeaderByTag(ctx context.Context, tag string) (*types.Header, error) {
	var header *types.Header
	if err := e.client.CallContext(ctx, &header, "eth_getBlockByNumber", tag, false); err != nil {
		return nil, err
	}
	return header, nil
}

func (e *engineAPI) GetHeaderByNumber(ctx context.Context, number uint64) (*types.Header, error) {
	var header *types.Header
	if err := e.client.CallContext(ctx, &header, "eth_getBlockByNumber", fmt.Sprintf("0x%x", number), false); err != nil {
		return nil, err
	}
	return header, nil
}
