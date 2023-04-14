package chainnode

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/infrastructure/network/rpcclient"
	"github.com/onemorebsmith/poolstratum/src/chainnode/aleo/aleostratum"
	I "github.com/onemorebsmith/poolstratum/src/comment"
	M "github.com/onemorebsmith/poolstratum/src/comment/model"
	// "github.com/onemorebsmith/poolstratum/src/gostratum"
	psm "github.com/onemorebsmith/poolstratum/src/prom"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var bigJobRegex = regexp.MustCompile(".*BzMiner.*")

// kaspa chain
type ChainKaspInterface interface {
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

// Aleo chain
const (
	ChainTypeAleo  = "aleo"
	ChainTypeKaspa = "kaspa"
)

const (
	ChannelId  = "aleopool"
	MinerName  = "rdpool"
	DeviceName = "rddevice"
)

type ChainAleoInterface interface {
	Subscribe() error
	Authorize() error
	Listen(cb func(line string) error) error
}

type ChainNodeInterface interface {
	ChainKaspInterface
	ChainAleoInterface
}

type ChainNode struct {
	chainType string
	Logger    *zap.SugaredLogger
	kasaip    *rpcclient.RPCClient
	aleoaip   *aleostratum.AleoStratumClient
}

func CreateChainNode(chainType string, address string, logger *zap.SugaredLogger) (*ChainNode, error) {
	var kasaip *rpcclient.RPCClient
	var aleoaip *aleostratum.AleoStratumClient
	var err error
	if chainType == ChainTypeKaspa {
		kasaip, err = rpcclient.NewRPCClient(address)
		if err != nil {
			return nil, err
		}
	}

	if chainType == ChainTypeAleo {
		aleoaip = aleostratum.CreateStratumClient(address, ChannelId, MinerName, DeviceName)
	}

	return &ChainNode{
		chainType: chainType,
		Logger:    logger.With(zap.String("client", chainType)),
		kasaip:    kasaip,
		aleoaip:   aleoaip,
	}, nil
}

func (chainnode *ChainNode) checkType(chainType string) bool {
	return chainnode.chainType == chainType
}

func (chainnode *ChainNode) Close() error {
	if chainnode.checkType(ChainTypeKaspa) {
		return chainnode.kasaip.Close()
	}
	return fmt.Errorf(chainnode.chainType, "  not Close")
}

func (chainnode *ChainNode) Reconnect() error {
	if chainnode.checkType(ChainTypeKaspa) {
		return chainnode.kasaip.Close()
	}
	return fmt.Errorf(chainnode.chainType, "  not Reconnect")
}

func (chainnode *ChainNode) GetBlockDAGInfo() (*appmessage.GetBlockDAGInfoResponseMessage, error) {
	if chainnode.checkType(ChainTypeKaspa) {
		return chainnode.kasaip.GetBlockDAGInfo()
	}
	return nil, fmt.Errorf(chainnode.chainType, "  not GetBlockDAGInfo")
}
func (chainnode *ChainNode) EstimateNetworkHashesPerSecond(startHash string, windowSize uint32) (*appmessage.EstimateNetworkHashesPerSecondResponseMessage, error) {
	if chainnode.checkType(ChainTypeKaspa) {
		return chainnode.kasaip.EstimateNetworkHashesPerSecond(startHash, windowSize)
	}
	return nil, fmt.Errorf(chainnode.chainType, "  not EstimateNetworkHashesPerSecond")
}
func (chainnode *ChainNode) GetInfo() (*appmessage.GetInfoResponseMessage, error) {
	if chainnode.checkType(ChainTypeKaspa) {
		return chainnode.kasaip.GetInfo()
	}
	return nil, fmt.Errorf(chainnode.chainType, "  not GetInfo")
}
func (chainnode *ChainNode) RegisterForNewBlockTemplateNotifications(onNewBlockTemplate func(notification *appmessage.NewBlockTemplateNotificationMessage)) error {
	if chainnode.checkType(ChainTypeKaspa) {
		return chainnode.kasaip.RegisterForNewBlockTemplateNotifications(onNewBlockTemplate)
	}
	return fmt.Errorf(chainnode.chainType, " not RegisterForNewBlockTemplateNotifications")
}
func (chainnode *ChainNode) GetBlockTemplate(miningAddress, extraData string) (*appmessage.GetBlockTemplateResponseMessage, error) {
	if chainnode.checkType(ChainTypeKaspa) {
		return chainnode.kasaip.GetBlockTemplate(miningAddress, extraData)
	}
	return nil, fmt.Errorf(chainnode.chainType, " not GetBlockTemplate")
}
func (chainnode *ChainNode) GetBalancesByAddresses(addresses []string) (*appmessage.GetBalancesByAddressesResponseMessage, error) {
	if chainnode.checkType(ChainTypeKaspa) {
		return chainnode.kasaip.GetBalancesByAddresses(addresses)
	}
	return nil, fmt.Errorf(chainnode.chainType, " not GetBalancesByAddresses")
}
func (chainnode *ChainNode) SubmitBlock(block *externalapi.DomainBlock) (appmessage.RejectReason, error) {
	if chainnode.checkType(ChainTypeKaspa) {
		return chainnode.kasaip.SubmitBlock(block)
	}
	return appmessage.RejectReasonNone, fmt.Errorf(chainnode.chainType, " not SubmitBlock")
}

func (chainnode *ChainNode) Subscribe() error {
	if chainnode.checkType(ChainTypeAleo) {
		return chainnode.aleoaip.Subscribe()
	}
	return fmt.Errorf(chainnode.chainType, " not Subscribe")
}

func (chainnode *ChainNode) Authorize() error {
	if chainnode.checkType(ChainTypeAleo) {
		return chainnode.aleoaip.Authorize()
	}
	return fmt.Errorf(chainnode.chainType, " not Authorize")
}

func (chainnode *ChainNode) Listen(cb func(line string) error) error {
	if chainnode.checkType(ChainTypeAleo) {
		return chainnode.aleoaip.Listen(cb)
	}
	return fmt.Errorf(chainnode.chainType, " not Listen")
}

// 保存chain发送的work数据
func (chainnode *ChainNode) SaveWork(work *M.JsonRpcEvent) error {
	if chainnode.checkType(ChainTypeAleo) {
		chainnode.aleoaip.LastWork = work
		return nil
	}
	return fmt.Errorf(chainnode.chainType, " not Listen")
}

func (chainnode *ChainNode) GetNotifyParams(diff float64, client I.WorkerClientInterface) (int, []any, error) {
	var jobId int
	var jobParams []any
	if chainnode.checkType(ChainTypeKaspa) {
		state := psm.GetMiningState(client)
		if client.WalletAddr() == "" {
			if time.Since(state.ConnectTime) > time.Second*20 { // timeout passed
				// this happens pretty frequently in gcp/aws land since script-kiddies scrape ports
				chainnode.Logger.Warn("client misconfigured, no miner address specified - disconnecting", zap.String("client", fmt.Sprintf(`%s/%s`, client.MinerName(), client.DeviceName())))
				psm.RecordWorkerError(client.WalletAddr(), psm.ErrNoMinerAddress)
				client.Disconnect() // invalid configuration, boot the worker
			}
			return 0, nil, fmt.Errorf("client wallet address is null")
		}
		template, err := chainnode.GetBlockTemplate(client.WalletAddr(), fmt.Sprintf(`'%s' client`, client.MinerName()))
		if err != nil {
			if strings.Contains(err.Error(), "Could not decode address") {
				psm.RecordWorkerError(client.WalletAddr(), psm.ErrInvalidAddressFmt)
				chainnode.Logger.Error(fmt.Sprintf("failed fetching new block template from kaspa, malformed address: %s", err))
				client.Disconnect() // unrecoverable
			} else {
				psm.RecordWorkerError(client.WalletAddr(), psm.ErrFailedBlockFetch)
				chainnode.Logger.Error(fmt.Sprintf("failed fetching new block template from kaspa: %s", err))
			}
			return 0, nil, err
		}
		state.BigDiff = psm.CalculateTarget(uint64(template.Block.Header.Bits))
		header, err := psm.SerializeBlockHeader(template.Block)
		if err != nil {
			psm.RecordWorkerError(client.WalletAddr(), psm.ErrBadDataFromMiner)
			chainnode.Logger.Error(fmt.Sprintf("failed to serialize block header: %s", err))
			return 0, nil, err
		}

		jobId = state.AddJob(template.Block)
		if !state.Initialized {
			state.Initialized = true
			state.UseBigJob = bigJobRegex.MatchString(client.MinerName())
			// first pass through send the difficulty since it's fixed
			state.StratumDiff = psm.NewKaspaDiff()
			state.StratumDiff.SetDiffValue(diff)
			if err := client.Send(M.JsonRpcEvent{
				Version: "2.0",
				Method:  "mining.set_difficulty",
				Params:  []any{state.StratumDiff.DiffValue},
			}); err != nil {
				psm.RecordWorkerError(client.WalletAddr(), psm.ErrFailedSetDiff)
				chainnode.Logger.Error(errors.Wrap(err, "failed sending difficulty").Error(), zap.Any("context", client))
				return 0, nil, err
			}
		}

		jobParams = []any{fmt.Sprintf("%d", jobId)}
		if state.UseBigJob {
			jobParams = append(jobParams, psm.GenerateLargeJobParams(header, uint64(template.Block.Header.Timestamp)))
		} else {
			jobParams = append(jobParams, psm.GenerateJobHeader(header))
			jobParams = append(jobParams, template.Block.Header.Timestamp)
		}

		return jobId, jobParams, nil
	} else if chainnode.checkType(ChainTypeAleo) {
		if chainnode.aleoaip.LastWork == nil {
			return 0, nil, fmt.Errorf(chainnode.chainType, " LastWork is nil")
		}
		if len(chainnode.aleoaip.LastWork.Params) != 6 {
			return 0, nil, fmt.Errorf(chainnode.chainType, " LastWork LastWork.Params len != 6")
		}
		jobId = 0
		jobParams = chainnode.aleoaip.LastWork.Params

		return jobId, jobParams, nil
	}
	return jobId, jobParams, fmt.Errorf(chainnode.chainType, " not Listen")
}
