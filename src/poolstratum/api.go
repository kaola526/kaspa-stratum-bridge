package poolstratum

import (
	"context"
	"fmt"
	"time"

	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/onemorebsmith/poolstratum/src/chainnode"
	M "github.com/onemorebsmith/poolstratum/src/comment/model"
	"github.com/onemorebsmith/poolstratum/src/gostratum"
	"github.com/onemorebsmith/poolstratum/src/prom"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type PoolApi struct {
	chainType     string
	address       string
	blockWaitTime time.Duration
	logger        *zap.SugaredLogger
	ChainNode     *chainnode.ChainNode
	connected     bool
}

func NewPoolAPI(chain_type string, address string, blockWaitTime time.Duration, logger *zap.SugaredLogger) (*PoolApi, error) {
	chainnode, err := chainnode.CreateChainNode(chain_type, address, logger)
	if err != nil {
		return nil, err
	}

	return &PoolApi{
		chainType:     chain_type,
		address:       address,
		blockWaitTime: blockWaitTime,
		logger:        logger.Named("[PoolApi]"),
		ChainNode:     chainnode,
		connected:     true,
	}, nil
}

func (api *PoolApi) Start(ctx context.Context, blockCb func()) {
	api.logger.Info("Start for ", api.chainType)
	if api.ChainNode.IsKaspa() {
		api.waitForSync(true)
		go api.startBlockTemplateListener(ctx, blockCb)
		go api.startStatsThread(ctx)
	} else if api.ChainNode.IsAleo() {
		api.ChainNode.Start(ctx, blockCb);
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
			dagResponse, err := api.ChainNode.GetBlockDAGInfo()
			if err != nil {
				api.logger.Warn("failed to get network hashrate from kaspa, prom stats will be out of date", zap.Error(err))
				continue
			}
			response, err := api.ChainNode.EstimateNetworkHashesPerSecond(dagResponse.TipHashes[0], 1000)
			if err != nil {
				api.logger.Warn("failed to get network hashrate from kaspa, prom stats will be out of date", zap.Error(err))
				continue
			}
			prom.RecordNetworkStats(response.NetworkHashesPerSecond, dagResponse.BlockCount, dagResponse.Difficulty)
		}
	}
}

func (api *PoolApi) reconnect() error {
	if api.ChainNode != nil {
		return api.ChainNode.Reconnect()
	}

	client, err := chainnode.CreateChainNode(api.chainType, api.address, api.logger)
	if err != nil {
		return err
	}
	api.ChainNode = client
	return nil
}

func (api *PoolApi) waitForSync(verbose bool) error {
	if verbose {
		api.logger.Info("checking kaspad sync state")
	}
	for {
		clientInfo, err := api.ChainNode.GetInfo()
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
	if api.chainType == chainnode.ChainTypeKaspa {
		blockReadyChan := make(chan bool)
		err := api.ChainNode.RegisterForNewBlockTemplateNotifications(func(_ *appmessage.NewBlockTemplateNotificationMessage) {
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
	} else if api.chainType == chainnode.ChainTypeAleo {
		api.ChainNode.Listen(func(line string) error {
			api.logger.Info("aleoclient Listen", line)
			event, err := M.UnmarshalEvent(line)
			if err != nil {
				api.logger.Error("error unmarshalling event", zap.String("raw", line))
				return err
			}
			api.ChainNode.SaveWork(&event)
			blockReadyCb()
			return nil
		})
	}
}

func (api *PoolApi) GetBlockTemplate(
	client *gostratum.WorkerContext) (*appmessage.GetBlockTemplateResponseMessage, error) {
	template, err := api.ChainNode.GetBlockTemplate(client.WalletAddr(),
		fmt.Sprintf(`'%s' via onemorebsmith/kaspa-stratum-bridge_%s`, client.MinerName, version))
	if err != nil {
		return nil, errors.Wrap(err, "failed fetching new block template from kaspa")
	}
	return template, nil
}
