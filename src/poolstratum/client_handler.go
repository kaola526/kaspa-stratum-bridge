package poolstratum

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/onemorebsmith/poolstratum/src/gostratum"
	"github.com/onemorebsmith/poolstratum/src/prom"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const balanceDelay = time.Minute

type clientListener struct {
	logger           *zap.SugaredLogger
	shareHandler     *shareHandler
	clientLock       sync.RWMutex
	clients          map[int32]*gostratum.StratumContext
	lastBalanceCheck time.Time
	clientCounter    int32
	minShareDiff     float64
	extranonceSize   int8
	maxExtranonce    int32
	nextExtranonce   int32
}

func newClientListener(logger *zap.SugaredLogger, shareHandler *shareHandler, minShareDiff float64, extranonceSize int8) *clientListener {
	return &clientListener{
		logger:         logger,
		minShareDiff:   minShareDiff,
		extranonceSize: extranonceSize,
		maxExtranonce:  int32(math.Pow(2, (8*math.Min(float64(extranonceSize), 3))) - 1),
		nextExtranonce: 0,
		clientLock:     sync.RWMutex{},
		shareHandler:   shareHandler,
		clients:        make(map[int32]*gostratum.StratumContext),
	}
}

func (c *clientListener) OnConnect(ctx *gostratum.StratumContext) {
	var extranonce int32
	// TODO，断开重连会+1，会不会存在问题
	// TODO，是否拒绝一些无效的连接
	idx := atomic.AddInt32(&c.clientCounter, 1)
	ctx.Id = idx
	c.clientLock.Lock()
	// nonce分配
	if c.extranonceSize > 0 {
		extranonce = c.nextExtranonce
		if c.nextExtranonce < c.maxExtranonce {
			c.nextExtranonce++
		} else {
			c.nextExtranonce = 0
			c.logger.Warn("wrapped extranonce! new clients may be duplicating work...")
		}
	}
	c.clients[idx] = ctx
	c.clientLock.Unlock()
	ctx.Logger = ctx.Logger.With(zap.Int("client_id", int(ctx.Id)))

	if c.extranonceSize > 0 {
		ctx.Extranonce = fmt.Sprintf("%0*x", c.extranonceSize*2, extranonce)
	}
	go func() {
		// hacky, but give time for the authorize to go through so we can use the worker name
		time.Sleep(5 * time.Second)
		c.shareHandler.getCreateStats(ctx) // create the stats if they don't exist
	}()
}

func (c *clientListener) OnDisconnect(ctx *gostratum.StratumContext) {
	ctx.Done()
	c.clientLock.Lock()
	c.logger.Info("removing client ", ctx.Id)
	delete(c.clients, ctx.Id)
	c.logger.Info("removed client ", ctx.Id)
	c.clientLock.Unlock()
	prom.RecordDisconnect(ctx)
}

func (c *clientListener) NewBlockAvailable(poolApi *PoolApi) {
	c.clientLock.Lock()
	addresses := make([]string, 0, len(c.clients))
	for _, cl := range c.clients {
		if cl.AppName != poolApi.chainType {
			c.logger.Info("client type ", cl.AppName, " != chain type "+poolApi.chainType)
			continue
		}
		if !cl.Connected() {
			continue
		}
		go func(client *gostratum.StratumContext) {
			jobId, jobParams, err := poolApi.ChainNode.GetNotifyParams(c.minShareDiff, client)
			if err != nil {
				return
			}

			// // normal notify flow
			if err := client.Send(gostratum.JsonRpcEvent{
				Version: "2.0",
				Method:  "mining.notify",
				Id:      jobId,
				Params:  jobParams,
			}); err != nil {
				if errors.Is(err, gostratum.ErrorDisconnected) {
					prom.RecordWorkerError(client.WalletAddr, prom.ErrDisconnected)
					return
				}
				prom.RecordWorkerError(client.WalletAddr, prom.ErrFailedSendWork)
				client.Logger.Error(errors.Wrapf(err, "failed sending work packet %d", jobId).Error())
			}

			prom.RecordNewJob(client)
		}(cl)

		if cl.WalletAddr != "" {
			addresses = append(addresses, cl.WalletAddr)
		}
	}
	c.clientLock.Unlock()

	if time.Since(c.lastBalanceCheck) > balanceDelay {
		c.lastBalanceCheck = time.Now()
		if len(addresses) > 0 {
			go func() {
				balances, err := poolApi.ChainNode.GetBalancesByAddresses(addresses)
				if err != nil {
					c.logger.Warn("failed to get balances from kaspa, prom stats will be out of date", zap.Error(err))
					return
				}
				prom.RecordBalances(balances)
			}()
		}
	}
}
