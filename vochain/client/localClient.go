package vochaincli

// From: https://github.com/tendermint/tendermint/blob/master/abci/client/local_client.go

/*
 * The main ABCI server (ie. non-GRPC) provides ordered asynchronous messages.
 * This is useful for DeliverTx and CheckTx, since it allows Tendermint to forward
 * transactions to the app before it's finished processing previous ones.
 * Thus, DeliverTx and CheckTx messages are sent asynchronously, while all other messages are sent synchronously.
 */

import (
	"sync"

	types "github.com/tendermint/tendermint/abci/types"
	cmn "github.com/tendermint/tendermint/libs/common"
)

var _ Client = (*LocalClient)(nil)

// NOTE: use defer to unlock mutex because Application might panic (e.g., in
// case of malicious tx or query). It only makes sense for publicly exposed
// methods like CheckTx (/broadcast_tx_* RPC endpoint) or Query (/abci_query
// RPC endpoint), but defers are used everywhere for the sake of consistency.
type LocalClient struct {
	cmn.BaseService

	mtx *sync.Mutex
	types.Application
	Callback
}

func NewLocalClient(mtx *sync.Mutex, app types.Application) *LocalClient {
	if mtx == nil {
		mtx = new(sync.Mutex)
	}
	LocalClient := &LocalClient{
		mtx:         mtx,
		Application: app,
	}
	LocalClient.BaseService = *cmn.NewBaseService(nil, "LocalClient", LocalClient)
	return LocalClient
}

func (app *LocalClient) SetResponseCallback(cb Callback) {
	app.mtx.Lock()
	app.Callback = cb
	app.mtx.Unlock()
}

// TODO: change types.Application to include Error()?
func (app *LocalClient) Error() error {
	return nil
}

func (app *LocalClient) DeliverTxAsync(params types.RequestDeliverTx) *ReqRes {
	app.mtx.Lock()
	defer app.mtx.Unlock()

	res := app.Application.DeliverTx(params)
	return app.callback(
		types.ToRequestDeliverTx(params),
		types.ToResponseDeliverTx(res),
	)
}

func (app *LocalClient) CheckTxAsync(req types.RequestCheckTx) *ReqRes {
	app.mtx.Lock()
	defer app.mtx.Unlock()

	res := app.Application.CheckTx(req)
	return app.callback(
		types.ToRequestCheckTx(req),
		types.ToResponseCheckTx(res),
	)
}

// Async
//-------------------------------------------------------

func (app *LocalClient) FlushSync() error {
	return nil
}

func (app *LocalClient) EchoSync(msg string) (*types.ResponseEcho, error) {
	return &types.ResponseEcho{Message: msg}, nil
}

func (app *LocalClient) InfoSync(req types.RequestInfo) (*types.ResponseInfo, error) {
	app.mtx.Lock()
	defer app.mtx.Unlock()

	res := app.Application.Info(req)
	return &res, nil
}

func (app *LocalClient) SetOptionSync(req types.RequestSetOption) (*types.ResponseSetOption, error) {
	app.mtx.Lock()
	defer app.mtx.Unlock()

	res := app.Application.SetOption(req)
	return &res, nil
}

func (app *LocalClient) QuerySync(req types.RequestQuery) (*types.ResponseQuery, error) {
	app.mtx.Lock()
	defer app.mtx.Unlock()

	res := app.Application.Query(req)
	return &res, nil
}

func (app *LocalClient) CommitSync() (*types.ResponseCommit, error) {
	app.mtx.Lock()
	defer app.mtx.Unlock()

	res := app.Application.Commit()
	return &res, nil
}

func (app *LocalClient) InitChainSync(req types.RequestInitChain) (*types.ResponseInitChain, error) {
	app.mtx.Lock()
	defer app.mtx.Unlock()

	res := app.Application.InitChain(req)
	return &res, nil
}

func (app *LocalClient) BeginBlockSync(req types.RequestBeginBlock) (*types.ResponseBeginBlock, error) {
	app.mtx.Lock()
	defer app.mtx.Unlock()

	res := app.Application.BeginBlock(req)
	return &res, nil
}

func (app *LocalClient) EndBlockSync(req types.RequestEndBlock) (*types.ResponseEndBlock, error) {
	app.mtx.Lock()
	defer app.mtx.Unlock()

	res := app.Application.EndBlock(req)
	return &res, nil
}

//-------------------------------------------------------

func (app *LocalClient) callback(req *types.Request, res *types.Response) *ReqRes {
	app.Callback(req, res)
	return newLocalReqRes(req, res)
}

func newLocalReqRes(req *types.Request, res *types.Response) *ReqRes {
	reqRes := NewReqRes(req)
	reqRes.Response = res
	reqRes.SetDone()
	return reqRes
}