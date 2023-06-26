package poolstratum

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	M "github.com/onemorebsmith/poolstratum/src/comment/model"
	"github.com/onemorebsmith/poolstratum/src/gostratum"
	"github.com/onemorebsmith/poolstratum/src/prom"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const balanceDelay = time.Minute

type workersAuthenticListener struct {
	logger           *zap.SugaredLogger
	shareHandler     *ShareHandler
	clientLock       sync.RWMutex
	clients          map[int32]*gostratum.WorkerContext
	lastBalanceCheck time.Time
	clientCounter    int32
	minShareDiff     float64
	extranonceSize   int8
	maxExtranonce    int32
	nextExtranonce   int32
	poolApi          *PoolApi
}

func newWorkersAuthenticListener(poolApi *PoolApi, logger *zap.SugaredLogger, shareHandler *ShareHandler, minShareDiff float64, extranonceSize int8) *workersAuthenticListener {
	return &workersAuthenticListener{
		logger:         logger.Named("[workersAuthenticListener]"),
		minShareDiff:   minShareDiff,
		extranonceSize: extranonceSize,
		maxExtranonce:  int32(math.Pow(2, (8*math.Min(float64(extranonceSize), 3))) - 1),
		nextExtranonce: 0,
		clientLock:     sync.RWMutex{},
		shareHandler:   shareHandler,
		clients:        make(map[int32]*gostratum.WorkerContext),
		poolApi:        poolApi,
	}
}

func (c *workersAuthenticListener) OnConnect(ctx *gostratum.WorkerContext) {
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

func (c *workersAuthenticListener) OnDisconnect(ctx *gostratum.WorkerContext) {
	ctx.Done()
	c.clientLock.Lock()
	c.logger.Info("removing client ", ctx.Id)
	delete(c.clients, ctx.Id)
	c.logger.Info("removed client ", ctx.Id)
	c.clientLock.Unlock()
	prom.RecordDisconnect(ctx)
}

func (c *workersAuthenticListener) NewBlockAvailable() {
	if c.poolApi == nil {
		return
	}
	poolApi := c.poolApi
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
		go func(client *gostratum.WorkerContext) {
			jobId, jobParams, err := poolApi.ChainNode.GetNotifyParams(c.minShareDiff, client)
			if err != nil {
				return
			}

			// // normal notify flow
			if err := client.Send(M.JsonRpcEvent{
				Version: "2.0",
				Method:  "mining.notify",
				Id:      jobId,
				Params:  jobParams,
			}); err != nil {
				if errors.Is(err, gostratum.ErrorDisconnected) {
					prom.RecordWorkerError(client.WalletAddr(), prom.ErrDisconnected)
					return
				}
				prom.RecordWorkerError(client.WalletAddr(), prom.ErrFailedSendWork)
				client.Logger.Error(errors.Wrapf(err, "failed sending work packet %d", jobId).Error())
			}

			prom.RecordNewJob(client)
		}(cl)

		if cl.WalletAddr() != "" {
			addresses = append(addresses, cl.WalletAddr())
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

func (c *workersAuthenticListener) ProxyHandlers(handlerMap gostratum.StratumHandlerMap) gostratum.StratumHandlerMap {

	return gostratum.StratumHandlerMap{
		string(M.StratumMethodSubscribe): c.HandleSubscribe,
		string(M.StratumMethodAuthorize): c.HandleAuthorize,
		string(M.StratumMethodSubmit):    handlerMap[string(M.StratumMethodSubmit)],
	}
}

func (c *workersAuthenticListener) HandleSubscribe(workerCtx *gostratum.WorkerContext, event M.JsonRpcEvent) error {
	c.logger.Info(fmt.Sprintf("HandleSubscribe event %v", event));
	if c.poolApi.ChainNode.IsAleo() {
		if err := workerCtx.Reply(M.NewResponse(event,
			[]any{workerCtx.Extranonce}, nil)); err != nil {
			return errors.Wrap(err, "failed to send response to subscribe")
		}
		if len(event.Params) > 0 {
			app, ok := event.Params[0].(string)
			if ok {
				workerCtx.AppName = app
			}
		}
		if len(event.Params) > 1 {
			version, ok := event.Params[1].(string)
			if ok {
				workerCtx.AppVersion = version
			}
		}
		if len(event.Params) > 2 {
			deviceCompany, ok := event.Params[2].(string)
			if ok {
				workerCtx.DeviceCompany = deviceCompany
			}
		}
		if len(event.Params) > 3 {
			deviceType, ok := event.Params[3].(string)
			if ok {
				workerCtx.DeviceType = deviceType
			}
		}
	}
	if c.poolApi.ChainNode.IsKaspa() {
		if err := workerCtx.Reply(M.NewResponse(event,
			[]any{true, "kaspa/1.0.0"}, nil)); err != nil {
			return errors.Wrap(err, "failed to send response to subscribe")
		}
		if len(event.Params) > 0 {
			app, ok := event.Params[0].(string)
			if ok {
				workerCtx.AppName = app
			}
		}
		if len(event.Params) > 1 {
			version, ok := event.Params[1].(string)
			if ok {
				workerCtx.AppVersion = version
			}
		}
		if len(event.Params) > 2 {
			deviceCompany, ok := event.Params[2].(string)
			if ok {
				workerCtx.DeviceCompany = deviceCompany
			}
		}
		if len(event.Params) > 3 {
			deviceType, ok := event.Params[3].(string)
			if ok {
				workerCtx.DeviceType = deviceType
			}
		}
	}
	return nil
}

func (c *workersAuthenticListener) HandleAuthorize(workerCtx *gostratum.WorkerContext, event M.JsonRpcEvent) error {
	c.logger.Info(fmt.Sprintf("HandleAuthorize event %v", event));
	return workerCtx.HandleAuthorize(event)
}
