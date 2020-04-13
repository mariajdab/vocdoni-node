package service

import (
	"time"

	voclient "github.com/tendermint/tendermint/rpc/client"
	"gitlab.com/vocdoni/go-dvote/census"
	"gitlab.com/vocdoni/go-dvote/config"
	"gitlab.com/vocdoni/go-dvote/crypto/signature"
	"gitlab.com/vocdoni/go-dvote/data"
	"gitlab.com/vocdoni/go-dvote/log"
	"gitlab.com/vocdoni/go-dvote/net"
	"gitlab.com/vocdoni/go-dvote/router"
	"gitlab.com/vocdoni/go-dvote/types"
	"gitlab.com/vocdoni/go-dvote/vochain/scrutinizer"
)

func API(apiconfig *config.API, pxy *net.Proxy, storage data.Storage, cm *census.Manager,
	sc *scrutinizer.Scrutinizer, vochainRPCaddr string, signer *signature.SignKeys) (err error) {
	log.Infof("creating API service")
	// API Endpoint initialization
	ws := new(net.WebsocketHandle)
	ws.Init(new(types.Connection))
	ws.SetProxy(pxy)

	listenerOutput := make(chan types.Message)
	go ws.Listen(listenerOutput)

	routerAPI := router.InitRouter(listenerOutput, storage, ws, signer, apiconfig.AllowPrivate)
	if apiconfig.File {
		log.Info("enabling file API")
		routerAPI.EnableFileAPI()
	}
	if apiconfig.Census {
		log.Info("enabling census API")
		routerAPI.EnableCensusAPI(cm)
	}
	if apiconfig.Vote {
		// creating the RPC calls client
		rpcClient, err := voclient.NewHTTP("tcp://"+vochainRPCaddr, "/websocket")
		if err != nil {
			log.Fatal(err)
		}
		// todo: client params as cli flags
		log.Info("enabling vote API")
		routerAPI.Scrutinizer = sc
		routerAPI.EnableVoteAPI(rpcClient)
	}

	go routerAPI.Route()
	ws.AddProxyHandler(apiconfig.Route)
	log.Infof("websockets API available at %s", apiconfig.Route)
	go func() {
		for {
			time.Sleep(60 * time.Second)
			log.Infof("[router info] privateReqs:%d publicReqs:%d", routerAPI.PrivateCalls, routerAPI.PublicCalls)
		}
	}()
	return
}