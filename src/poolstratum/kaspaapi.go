package poolstratum

import (
	"context"
	"fmt"
	"time"

	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/infrastructure/network/rpcclient"
	"github.com/onemorebsmith/poolstratum/src/chainnode/aleo/aleostratum"
	"github.com/onemorebsmith/poolstratum/src/gostratum"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	ChainTypeAleo  = "aleo"
	ChainTypeKaspa = "kaspa"
)

const (
	ChannelId  = "aleopool"
	MinerName  = "rdpool"
	DeviceName = "rddevice"
)

// 定义一个接口
type ChainApiInterface interface {
	Reconnect() error
	Close() error
	GetBlockDAGInfo() (*appmessage.GetBlockDAGInfoResponseMessage, error)
	EstimateNetworkHashesPerSecond(startHash string, windowSize uint32) (*appmessage.EstimateNetworkHashesPerSecondResponseMessage, error)
	GetInfo() (*appmessage.GetInfoResponseMessage, error)
	RegisterForNewBlockTemplateNotifications(onNewBlockTemplate func(notification *appmessage.NewBlockTemplateNotificationMessage)) error
	GetBlockTemplate(miningAddress, extraData string) (*appmessage.GetBlockTemplateResponseMessage, error)
	GetBalancesByAddresses(addresses []string) (*appmessage.GetBalancesByAddressesResponseMessage, error)
	SubmitBlock(block *externalapi.DomainBlock) (appmessage.RejectReason, error)
}

type PoolApi struct {
	chainType     string
	address       string
	blockWaitTime time.Duration
	logger        *zap.SugaredLogger
	chainapi      ChainApiInterface
	connected     bool
}

func NewPoolAPI(chain_type string, address string, blockWaitTime time.Duration, logger *zap.SugaredLogger) (*PoolApi, error) {
	var chainapi ChainApiInterface
	var err error
	if chain_type == ChainTypeKaspa {
		chainapi, err = rpcclient.NewRPCClient(address)
		if err != nil {
			return nil, err
		}
	}

	if chain_type == ChainTypeAleo {
		chainapi = aleostratum.CreateStratumClient(address, ChannelId, MinerName, DeviceName)
	}

	return &PoolApi{
		chainType:     chain_type,
		address:       address,
		blockWaitTime: blockWaitTime,
		logger:        logger.With(zap.String("component", chain_type+":"+address)),
		chainapi:      chainapi,
		connected:     true,
	}, nil
}

func (api *PoolApi) Start(ctx context.Context, blockCb func()) {
	fmt.Print(api.chainType, " Start\n")
	if api.chainType == ChainTypeKaspa {
		api.waitForSync(true)
		go api.startBlockTemplateListener(ctx, blockCb)
		go api.startStatsThread(ctx)
	} else if api.chainType == ChainTypeAleo {
		go func(ctx context.Context, blockCb func()) {
			// TODO
			// for {
			// 	fmt.Print("AleoNode Subscribe\n")
			// 	err := api.aleop.Subscribe()
			// 	if err != nil {
			// 		api.logger.Error("subscribe err ", err)
			// 		time.Sleep(time.Second * 5)
			// 		continue
			// 	}
			// 	fmt.Print("AleoNode Authorize\n")
			// 	err = api.aleop.Authorize()
			// 	if err != nil {
			// 		api.logger.Error("authorize err ", err)
			// 		time.Sleep(time.Second * 5)
			// 		continue
			// 	}

			// 	api.startBlockTemplateListener(ctx, blockCb)
			// 	time.Sleep(time.Second * 5)
			// }
		}(ctx, blockCb)
	}
}

func (api *PoolApi) startStatsThread(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-ctx.Done():
			api.logger.Warn("context cancelled, stopping stats thread")
			return
		case <-ticker.C:
			dagResponse, err := api.chainapi.GetBlockDAGInfo()
			if err != nil {
				api.logger.Warn("failed to get network hashrate from kaspa, prom stats will be out of date", zap.Error(err))
				continue
			}
			response, err := api.chainapi.EstimateNetworkHashesPerSecond(dagResponse.TipHashes[0], 1000)
			if err != nil {
				api.logger.Warn("failed to get network hashrate from kaspa, prom stats will be out of date", zap.Error(err))
				continue
			}
			RecordNetworkStats(response.NetworkHashesPerSecond, dagResponse.BlockCount, dagResponse.Difficulty)
		}
	}
}

func (api *PoolApi) reconnect() error {
	if api.chainapi != nil {
		return api.chainapi.Reconnect()
	}

	client, err := rpcclient.NewRPCClient(api.address)
	if err != nil {
		return err
	}
	api.chainapi = client
	return nil
}

func (api *PoolApi) waitForSync(verbose bool) error {
	if verbose {
		api.logger.Info("checking kaspad sync state")
	}
	for {
		clientInfo, err := api.chainapi.GetInfo()
		if err != nil {
			return errors.Wrapf(err, "error fetching server info from kaspad @ %s", api.address)
		}
		if clientInfo.IsSynced {
			break
		}
		api.logger.Warn("Kaspa is not synced, waiting for sync before starting bridge")
		time.Sleep(5 * time.Second)
	}
	if verbose {
		api.logger.Info("kaspad synced, starting server")
	}
	return nil
}

func (api *PoolApi) startBlockTemplateListener(ctx context.Context, blockReadyCb func()) {
	if api.chainType == ChainTypeKaspa {
		blockReadyChan := make(chan bool)
		err := api.chainapi.RegisterForNewBlockTemplateNotifications(func(_ *appmessage.NewBlockTemplateNotificationMessage) {
			blockReadyChan <- true
		})
		if err != nil {
			api.logger.Error("fatal: failed to register for block notifications from kaspa")
		}

		ticker := time.NewTicker(api.blockWaitTime)
		for {
			if err := api.waitForSync(false); err != nil {
				api.logger.Error("error checking kaspad sync state, attempting reconnect: ", err)
				if err := api.reconnect(); err != nil {
					api.logger.Error("error reconnecting to kaspad, waiting before retry: ", err)
					time.Sleep(5 * time.Second)
				}
			}
			select {
			case <-ctx.Done():
				api.logger.Warn("context cancelled, stopping block update listener")
				return
			case <-blockReadyChan:
				blockReadyCb()
				ticker.Reset(api.blockWaitTime)
			case <-ticker.C: // timeout, manually check for new blocks
				blockReadyCb()
			}
		}
	} else if api.chainType == ChainTypeAleo {
		// TODO
		// api.aleop.Listen(func(line string) error {
		// 	fmt.Println("aleoclient Listen", line)
		// 	return nil
		// })
	}
}

func (api *PoolApi) GetBlockTemplate(
	client *gostratum.StratumContext) (*appmessage.GetBlockTemplateResponseMessage, error) {
	template, err := api.chainapi.GetBlockTemplate(client.WalletAddr,
		fmt.Sprintf(`'%s' via onemorebsmith/kaspa-stratum-bridge_%s`, client.MinerName, version))
	if err != nil {
		return nil, errors.Wrap(err, "failed fetching new block template from kaspa")
	}
	return template, nil
}
