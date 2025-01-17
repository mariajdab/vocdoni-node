package service

import (
	"context"
	"os"
	"time"

	"go.vocdoni.io/dvote/config"
	"go.vocdoni.io/dvote/data"
	"go.vocdoni.io/dvote/ipfsconnect"
	"go.vocdoni.io/dvote/log"
)

func (vs *VocdoniService) IPFS(ipfsconfig *config.IPFSCfg) (storage data.Storage, err error) {
	log.Info("creating ipfs service")
	os.Setenv("IPFS_FD_MAX", "1024")
	ipfsStore := data.IPFSNewConfig(ipfsconfig.ConfigPath)
	storage, err = data.Init(data.StorageIDFromString("IPFS"), ipfsStore)
	if err != nil {
		return
	}

	go func() {
		for {
			time.Sleep(time.Second * 120)
			tctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			log.Monitor("ipfs storage", storage.Stats(tctx))
			cancel()
		}
	}()

	go storage.CollectMetrics(context.Background(), vs.MetricsAgent)

	if len(ipfsconfig.ConnectKey) > 0 {
		log.Info("enabling ipfsconnect cluster")
		_, priv := vs.Signer.HexString()
		ipfsconn := ipfsconnect.New(
			ipfsconfig.ConnectKey,
			priv,
			"libp2p",
			storage,
		)
		if len(ipfsconfig.ConnectPeers) > 0 && len(ipfsconfig.ConnectPeers[0]) > 8 {
			log.Debugf("using custom ipfsconnect bootnodes %s", ipfsconfig.ConnectPeers)
			ipfsconn.Transport.BootNodes = ipfsconfig.ConnectPeers
		}
		ipfsconn.Start()
	}
	return
}
