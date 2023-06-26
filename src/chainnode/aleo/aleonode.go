package aleo

import (
	"context"
	"fmt"
	"time"

	"github.com/onemorebsmith/poolstratum/src/chainnode/aleo/aleostratum"
	M "github.com/onemorebsmith/poolstratum/src/comment/model"
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
	aleoclient *aleostratum.AleoStratumClient
}

func NewAleoNode(address string, blockWaitTime time.Duration, logger *zap.SugaredLogger) (*AleoNode, error) {
	client := aleostratum.CreateStratumClient(address, ChannelId, MinerName, DeviceName)

	return &AleoNode{
		address:       address,
		blockWaitTime: blockWaitTime,
		logger:        logger.Named("[NewAleoNode]"),
		aleoclient:    client,
		connected:     true,
	}, nil
}

func (ks *AleoNode) Start(ctx context.Context, blockCb func()) {
	ks.logger.Info("AleoNode Start")
	go func(ctx context.Context, blockCb func()) {
		for {
			ks.logger.Info("AleoNode Subscribe")
			err := ks.aleoclient.Subscribe()
			if err != nil {
				ks.logger.Error("subscribe err ", err)
				time.Sleep(time.Second * 5)
				continue
			}
			ks.logger.Info("AleoNode Authorize\n")
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

func (ks *AleoNode) Subscribe() error {
	return ks.aleoclient.Subscribe()
}

func (ks *AleoNode) Authorize() error {
	return ks.aleoclient.Authorize()
}

func (ks *AleoNode) Listen(cb aleostratum.LineCallback) error {
	return ks.aleoclient.Listen(cb)
}

func (ks *AleoNode) SetLastWork(work *M.JsonRpcEvent) {
	ks.aleoclient.LastWork = work;
}

func (ks *AleoNode) GetLastWork() *M.JsonRpcEvent{
	return ks.aleoclient.LastWork;
}