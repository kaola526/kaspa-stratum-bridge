package aleo

import (
	"context"
	"fmt"
	"time"

	"github.com/onemorebsmith/poolstratum/src/chainnode/aleo/aleostratum"
	"go.uber.org/zap"
)

const (
	ChannelId  = "aleopool"
	MinerName  = "rdpool"
	DeviceName = "rddevice"
)

type AleoNode struct {
	/// aleo pool ip:port
	address string
	/// block wait time
	blockWaitTime time.Duration
	/// 日志句柄
	logger *zap.SugaredLogger
	/// 连接状态
	connected bool
	///
	aleoclient aleostratum.StratumClient
}

func NewAleoNode(address string, blockWaitTime time.Duration, logger *zap.SugaredLogger) (*AleoNode, error) {
	client := aleostratum.CreateStratumClient(address, ChannelId, MinerName, DeviceName)

	return &AleoNode{
		address:       address,
		blockWaitTime: blockWaitTime,
		logger:        logger.With(zap.String("component", "aleo:"+address)),
		aleoclient:    client,
		connected:     true,
	}, nil
}

func (ks *AleoNode) Start(ctx context.Context, blockCb func()) {
	fmt.Print("AleoNode Start")
	go func(ctx context.Context, blockCb func()) {
		for {
			fmt.Print("AleoNode Subscribe\n")
			err := ks.aleoclient.Subscribe()
			if err != nil {
				ks.logger.Error("subscribe err ", err)
				time.Sleep(time.Second * 5)
				continue
			}
			fmt.Print("AleoNode Authorize\n")
			err = ks.aleoclient.Authorize()
			if err != nil {
				ks.logger.Error("authorize err ", err)
				time.Sleep(time.Second * 5)
				continue
			}

			ks.startBlockTemplateListener(ctx, blockCb)
			time.Sleep(time.Second * 5)
		}
	}(ctx, blockCb)
}

func (ks *AleoNode) startBlockTemplateListener(ctx context.Context, blockCb func()) {
	ks.aleoclient.Listen(func(line string) error {
		fmt.Println("aleoclient Listen", line)
		return nil
	})
}
