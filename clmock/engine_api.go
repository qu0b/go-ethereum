package clmock

import (
	"context"
	"time"

	"net/http"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/rpc"
)

type engineAPI struct {
	client *rpc.Client
}

func (e *engineAPI) Connect(ctx context.Context, httpEndpoint string) error {
	httpEndpoint = "http://127.0.0.1:8545"
	client, err := rpc.DialOptions(ctx, httpEndpoint, rpc.WithHTTPClient(&http.Client{
                Timeout: 10 * time.Second,
        }))

	if err != nil {
		return err
	}
	e.client = client
	return nil
}

func (e *engineAPI) ForkchoiceUpdatedV1(ctx context.Context, fcState *engine.ForkchoiceStateV1, payloadAttr *engine.PayloadAttributes) (*engine.ForkChoiceResponse, error) {
	var resp engine.ForkChoiceResponse
	if err := e.client.CallContext(ctx, &resp, "engine_forkchoiceUpdatedV1", fcState, payloadAttr); err != nil {
		return nil, err
	}
	return &resp, nil
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
	if err := e.client.CallContext(ctx, &header, "eth_getBlockByNumber", tag); err != nil {
		return nil, err
	}
	return header, nil
}

func (e *engineAPI) GetHeaderByNumber(ctx context.Context, number uint64) (*types.Header, error) {
	var header *types.Header
	if err := e.client.CallContext(ctx, &header, "eth_getBlockByNumber", number); err != nil {
		return nil, err
	}
	return header, nil
}
