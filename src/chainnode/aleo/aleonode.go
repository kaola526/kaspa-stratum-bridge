package aleo

import (
	"context"
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
	aleoclient *aleostratum.AleoStratumNode
}

func NewAleoNode(address string, blockWaitTime time.Duration, logger *zap.SugaredLogger) (*AleoNode, error) {
	client := aleostratum.CreateStratumNode(address, ChannelId, MinerName, DeviceName, logger)

	return &AleoNode{
		address:       address,
		blockWaitTime: blockWaitTime,
		logger:        logger.Named("[AleoNode]"),
		aleoclient:    client,
		connected:     true,
	}, nil
}

func (aleoNode *AleoNode) Start(ctx context.Context, blockCb func()) {
	aleoNode.logger.Info("Start")
	go func(ctx context.Context, blockCb func()) {
		for {
			aleoNode.logger.Info("Start->Subscribe")
			err := aleoNode.aleoclient.Subscribe()
			if err != nil {
				aleoNode.logger.Error("subscribe err ", err)
				time.Sleep(time.Second * 5)
				continue
			}
			aleoNode.logger.Info("Start->Authorize")
			err = aleoNode.aleoclient.Authorize()
			if err != nil {
				aleoNode.logger.Error("authorize err ", err)
				time.Sleep(time.Second * 5)
				continue
			}

			aleoNode.logger.Info("Start->startBlockTemplateListener")
			aleoNode.startBlockTemplateListener(ctx, blockCb)
			time.Sleep(time.Second * 5)
		}
	}(ctx, blockCb)
}

func (aleoNode *AleoNode) startBlockTemplateListener(ctx context.Context, blockCb func()) {
	aleoNode.logger.Info("startBlockTemplateListener")
	aleoNode.aleoclient.Listen(func(line string) error {
		aleoNode.logger.Info("Listen line ", line)
		event, err := M.UnmarshalEvent(line)
		if err != nil {
			aleoNode.logger.Error("startBlockTemplateListener cb err ", err)
		}
		aleoNode.SetLastWork(&event)
		blockCb()
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
	ks.aleoclient.LastWork = work
}

func (ks *AleoNode) GetLastWork() *M.JsonRpcEvent {
	return ks.aleoclient.LastWork
}
