package poolstratum

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/domain/consensus/utils/consensushashing"
	"github.com/kaspanet/kaspad/domain/consensus/utils/pow"
	"github.com/onemorebsmith/poolstratum/src/chainnode"
	"github.com/onemorebsmith/poolstratum/src/gostratum"
	"github.com/onemorebsmith/poolstratum/src/mq"
	"github.com/onemorebsmith/poolstratum/src/prom"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type WorkStats struct {
	BlocksFound   atomic.Int64
	SharesFound   atomic.Int64
	SharesDiff    atomic.Float64
	StaleShares   atomic.Int64
	InvalidShares atomic.Int64
	AppNameOrVer  string
	DeviceName    string
	StartTime     time.Time
	LastShare     time.Time
}

type shareHandler struct {
	poolapi      *chainnode.ChainNode
	stats        map[string]*WorkStats
	statsLock    sync.Mutex
	overall      WorkStats
	tipBlueScore uint64
}

func newShareHandler(poolapi *chainnode.ChainNode) *shareHandler {
	return &shareHandler{
		poolapi:   poolapi,
		stats:     map[string]*WorkStats{},
		statsLock: sync.Mutex{},
	}
}

func (sh *shareHandler) getCreateStats(ctx *gostratum.StratumContext) *WorkStats {
	sh.statsLock.Lock()
	var stats *WorkStats
	found := false
	if ctx.DeviceName != "" {
		stats, found = sh.stats[ctx.DeviceName]
	}
	if !found { // no worker name, check by remote address
		stats, found = sh.stats[ctx.RemoteAddr]
		if found {
			// no worker name, but remote addr is there
			// so replacet the remote addr with the worker names
			// TOOD，代码存在问题
			delete(sh.stats, ctx.RemoteAddr)
			stats.DeviceName = ctx.DeviceName
			sh.stats[ctx.DeviceName] = stats
		}
	}
	if !found { // legit doesn't exist, create it
		stats = &WorkStats{}
		stats.LastShare = time.Now()
		stats.DeviceName = ctx.RemoteAddr
		stats.StartTime = time.Now()
		sh.stats[ctx.RemoteAddr] = stats

		// TODO: not sure this is the best place, nor whether we shouldn't be
		// resetting on disconnect
		prom.InitWorkerCounters(ctx)
	}
	stats.AppNameOrVer = ctx.AppName + "/" + ctx.AppVersion
	sh.statsLock.Unlock()
	return stats
}

type submitInfo struct {
	block    *appmessage.RPCBlock
	state    *prom.MiningState
	Noncestr string
	NonceVal uint64
}

func validateSubmit(ctx *gostratum.StratumContext, event gostratum.JsonRpcEvent) (*submitInfo, error) {
	if len(event.Params) < 3 {
		prom.RecordWorkerError(ctx.WalletAddr, prom.ErrBadDataFromMiner)
		return nil, fmt.Errorf("malformed event, expected at least 2 params")
	}
	jobIdStr, ok := event.Params[1].(string)
	if !ok {
		prom.RecordWorkerError(ctx.WalletAddr, prom.ErrBadDataFromMiner)
		return nil, fmt.Errorf("unexpected type for param 1: %+v", event.Params...)
	}
	jobId, err := strconv.ParseInt(jobIdStr, 10, 0)
	if err != nil {
		prom.RecordWorkerError(ctx.WalletAddr, prom.ErrBadDataFromMiner)
		return nil, errors.Wrap(err, "job id is not parsable as an number")
	}
	state := prom.GetMiningState(ctx)
	block, exists := state.GetJob(int(jobId))
	if !exists {
		prom.RecordWorkerError(ctx.WalletAddr, prom.ErrMissingJob)
		return nil, fmt.Errorf("job does not exist. stale?")
	}
	noncestr, ok := event.Params[2].(string)
	if !ok {
		prom.RecordWorkerError(ctx.WalletAddr, prom.ErrBadDataFromMiner)
		return nil, fmt.Errorf("unexpected type for param 2: %+v", event.Params...)
	}
	return &submitInfo{
		state:    state,
		block:    block,
		Noncestr: strings.Replace(noncestr, "0x", "", 1),
	}, nil
}

var (
	ErrStaleShare = fmt.Errorf("stale share")
	ErrDupeShare  = fmt.Errorf("duplicate share")
)

// the max difference between tip blue score and job blue score that we'll accept
// anything greater than this is considered a stale
const workWindow = 8

func (sh *shareHandler) checkStales(ctx *gostratum.StratumContext, si *submitInfo) error {
	tip := sh.tipBlueScore
	if si.block.Header.BlueScore > tip {
		sh.tipBlueScore = si.block.Header.BlueScore
		return nil // can't be
	}
	if tip-si.block.Header.BlueScore > workWindow {
		prom.RecordStaleShare(ctx)
		return errors.Wrapf(ErrStaleShare, "blueScore %d vs %d", si.block.Header.BlueScore, tip)
	}
	// TODO (bs): dupe share tracking
	return nil
}

func (sh *shareHandler) HandleSubmit(ctx *gostratum.StratumContext, event gostratum.JsonRpcEvent) error {
	submitInfo, err := validateSubmit(ctx, event)
	if err != nil {
		return err
	}

	// add extranonce to noncestr if enabled and submitted nonce is shorter than
	// expected (16 - <extranonce length> characters)
	if ctx.Extranonce != "" {
		extranonce2Len := 16 - len(ctx.Extranonce)
		if len(submitInfo.Noncestr) <= extranonce2Len {
			submitInfo.Noncestr = ctx.Extranonce + fmt.Sprintf("%0*s", extranonce2Len, submitInfo.Noncestr)
		}
	}

	//ctx.Logger.Debug(submitInfo.block.Header.BlueScore, " submit ", submitInfo.noncestr)
	// TODO：bug
	state := prom.GetMiningState(ctx)
	if state.UseBigJob {
		submitInfo.NonceVal, err = strconv.ParseUint(submitInfo.Noncestr, 16, 64)
		if err != nil {
			prom.RecordWorkerError(ctx.WalletAddr, prom.ErrBadDataFromMiner)
			return errors.Wrap(err, "failed parsing noncestr")
		}
	} else {
		submitInfo.NonceVal, err = strconv.ParseUint(submitInfo.Noncestr, 16, 64)
		if err != nil {
			prom.RecordWorkerError(ctx.WalletAddr, prom.ErrBadDataFromMiner)
			return errors.Wrap(err, "failed parsing noncestr")
		}
	}
	stats := sh.getCreateStats(ctx)
	// if err := sh.checkStales(ctx, submitInfo); err != nil {
	// 	if err == ErrDupeShare {
	// 		ctx.Logger.Info("dupe share "+submitInfo.noncestr, ctx.DeviceName, ctx.WalletAddr)
	// 		atomic.AddInt64(&stats.StaleShares, 1)
	// 		RecordDupeShare(ctx)
	// 		return ctx.ReplyDupeShare(event.Id)
	// 	} else if errors.Is(err, ErrStaleShare) {
	// 		ctx.Logger.Info(err.Error(), ctx.DeviceName, ctx.WalletAddr)
	// 		atomic.AddInt64(&stats.StaleShares, 1)
	// 		RecordStaleShare(ctx)
	// 		return ctx.ReplyStaleShare(event.Id)
	// 	}
	// 	// unknown error somehow
	// 	ctx.Logger.Error("unknown error during check stales: ", err.Error())
	// 	return ctx.ReplyBadShare(event.Id)
	// }

	converted, err := appmessage.RPCBlockToDomainBlock(submitInfo.block)
	if err != nil {
		return fmt.Errorf("failed to cast block to mutable block: %+v", err)
	}
	mutableHeader := converted.Header.ToMutable()
	mutableHeader.SetNonce(submitInfo.NonceVal)
	powState := pow.NewState(mutableHeader)
	powValue := powState.CalculateProofOfWorkValue()

	// The block hash must be less or equal than the claimed target.
	if powValue.Cmp(&powState.Target) <= 0 {
		if err := sh.submit(ctx, converted, submitInfo.NonceVal, event.Id); err != nil {
			return err
		}
	}
	// remove for now until I can figure it out. No harm here as we're not
	// } else if powValue.Cmp(state.stratumDiff.targetValue) >= 0 {
	// 	ctx.Logger.Warn("weak block")
	// 	RecordWeakShare(ctx)
	// 	return ctx.ReplyLowDiffShare(event.Id)
	// }

	// TODO：
	stats.SharesFound.Add(1)
	stats.SharesDiff.Add(state.StratumDiff.HashValue)
	stats.LastShare = time.Now()
	sh.overall.SharesFound.Add(1)
	prom.RecordShareFound(ctx, state.StratumDiff.HashValue)

	params, _ := json.Marshal(submitInfo)

	mqDate := mq.MQShareRecordData{
		MessageId:        uuid.New().String(),
		AppName:          ctx.AppName,
		AppVersion:       ctx.AppVersion,
		RecodeType:       "submit",
		MinerName:        ctx.MinerName,
		DeviceCompany:    ctx.DeviceCompany,
		DeviceType:       ctx.DeviceType,
		DeviceName:       ctx.DeviceName,
		RemoteAddr:       ctx.RemoteAddr,
		Time:             time.Now().UnixNano() / int64(time.Millisecond),
		Code:             0,
		TargetDifficulty: uint64(state.StratumDiff.DiffValue),
		Params:           string(params),
	}

	jsonData, err := json.MarshalIndent(mqDate, "", "  ")
	if err == nil {
		mq.Insertmqqt(ctx, string(jsonData), "Kaspa_Direct_Exchange", "Kaspa_Direct_Routing")
	}

	return ctx.Reply(gostratum.JsonRpcResponse{
		Id:     event.Id,
		Result: true,
	})
}

func (sh *shareHandler) submit(ctx *gostratum.StratumContext,
	block *externalapi.DomainBlock, nonce uint64, eventId any) error {
	mutable := block.Header.ToMutable()
	mutable.SetNonce(nonce)
	block = &externalapi.DomainBlock{
		Header:       mutable.ToImmutable(),
		Transactions: block.Transactions,
	}
	_, err := sh.poolapi.SubmitBlock(block)
	blockhash := consensushashing.BlockHash(block)
	// print after the submit to get it submitted faster
	ctx.Logger.Info(fmt.Sprintf("Submitted block %s", blockhash))

	if err != nil {
		// :'(
		if strings.Contains(err.Error(), "ErrDuplicateBlock") {
			ctx.Logger.Warn("block rejected, stale")
			// stale
			sh.getCreateStats(ctx).StaleShares.Add(1)
			sh.overall.StaleShares.Add(1)
			prom.RecordStaleShare(ctx)
			return ctx.ReplyStaleShare(eventId)
		} else {
			ctx.Logger.Warn("block rejected, unknown issue (probably bad pow", zap.Error(err))
			sh.getCreateStats(ctx).InvalidShares.Add(1)
			sh.overall.InvalidShares.Add(1)
			prom.RecordInvalidShare(ctx)
			return ctx.ReplyBadShare(eventId)
		}
	}

	// :)
	ctx.Logger.Info(fmt.Sprintf("block accepted %s", blockhash))
	stats := sh.getCreateStats(ctx)
	stats.BlocksFound.Add(1)
	sh.overall.BlocksFound.Add(1)
	prom.RecordBlockFound(ctx, block.Header.Nonce(), block.Header.BlueScore(), blockhash.String())

	// nil return allows HandleSubmit to record share (blocks are shares too!) and
	// handle the response to the client
	return nil
}

func (sh *shareHandler) startStatsThread() error {
	start := time.Now()
	for {
		// console formatting is terrible. Good luck whever touches anything
		time.Sleep(10 * time.Second)
		sh.statsLock.Lock()
		str := "\n=================================================================================================\n"
		str += "     app name    |  worker name   |  avg hashrate  |   acc/stl/inv  |    blocks    |    uptime   \n"
		str += "-------------------------------------------------------------------------------------------------\n"
		var lines []string
		totalRate := float64(0)
		for _, v := range sh.stats {
			rate := GetAverageHashrateGHs(v)
			totalRate += rate
			rateStr := fmt.Sprintf("%0.2fGH/s", rate) // todo, fix units
			ratioStr := fmt.Sprintf("%d/%d/%d", v.SharesFound.Load(), v.StaleShares.Load(), v.InvalidShares.Load())
			lines = append(lines, fmt.Sprintf(" %-16s| %-15s| %14.14s | %14.14s | %12d | %11s",
				v.AppNameOrVer, v.DeviceName, rateStr, ratioStr, v.BlocksFound.Load(), time.Since(v.StartTime).Round(time.Second)))
		}
		sort.Strings(lines)
		str += strings.Join(lines, "\n")
		rateStr := fmt.Sprintf("%0.2fGH/s", totalRate) // todo, fix units
		ratioStr := fmt.Sprintf("%d/%d/%d", sh.overall.SharesFound.Load(), sh.overall.StaleShares.Load(), sh.overall.InvalidShares.Load())
		str += "\n-------------------------------------------------------------------------------------------------\n"
		str += fmt.Sprintf("                 |                | %14.14s | %14.14s | %12d | %11s",
			rateStr, ratioStr, sh.overall.BlocksFound.Load(), time.Since(start).Round(time.Second))
		str += "\n========================================================================== pool_bridge_" + version + " ===\n"
		sh.statsLock.Unlock()
		log.Println(str)
	}
}

func GetAverageHashrateGHs(stats *WorkStats) float64 {
	return stats.SharesDiff.Load() / time.Since(stats.StartTime).Seconds()
}
