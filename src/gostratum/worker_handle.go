package gostratum

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	util "github.com/onemorebsmith/poolstratum/src/chainnode"
	M "github.com/onemorebsmith/poolstratum/src/comment/model"
	"github.com/onemorebsmith/poolstratum/src/mq"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func (workerCtx *WorkerContext) HandleSubscribe(event M.JsonRpcEvent) error {
	workerCtx.Logger.Info(fmt.Sprintf("HandleSubscribe event %v", event))
	if workerCtx.IsAleo() {
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
	if workerCtx.IsKaspa() {
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

func (workerCtx *WorkerContext) HandleAuthorize(event M.JsonRpcEvent) error {
	workerCtx.Logger.Info(fmt.Sprintf("HandleAuthorize event %v", event))
	if len(event.Params) < 2 {
		return fmt.Errorf("malformed event from miner, expected param[1] to be address")
	}
	minername, ok := event.Params[0].(string)
	if !ok {
		return fmt.Errorf("malformed event from miner, expected param[1] to be address string")
	}

	devicename, ok := event.Params[1].(string)
	if !ok {
		return fmt.Errorf("malformed event from miner, expected param[1] to be address string")
	}

	var err error
	address, err := util.CleanWallet(workerCtx.AppName, "kaspa:qzn4fltcsh30n22f6zszvuy9pkzjnmz97dcvm740wd5l98dqw94q6s820ggvg")
	if err != nil {
		return fmt.Errorf("invalid wallet format %s: %w", address, err)
	}

	workerCtx.walletAddr = address
	workerCtx.minerName = minername
	workerCtx.deviceName = devicename
	workerCtx.Logger = workerCtx.Logger.With(zap.String("worker", workerCtx.deviceName), zap.String("addr", workerCtx.walletAddr))

	if err := workerCtx.Reply(M.NewResponse(event, true, nil)); err != nil {
		return errors.Wrap(err, "failed to send response to authorize")
	}
	if workerCtx.Extranonce != "" {
		SendExtranonce(workerCtx)
	}

	mqData := mq.MQShareRecordData{
		MessageId:     uuid.New().String(),
		AppName:       workerCtx.AppName,
		AppVersion:    workerCtx.AppVersion,
		RecodeType:    "Login",
		MinerName:     workerCtx.minerName,
		DeviceCompany: workerCtx.DeviceCompany,
		DeviceType:    workerCtx.DeviceType,
		DeviceName:    workerCtx.deviceName,
		RemoteAddr:    workerCtx.remoteAddr,
		Time:          time.Now().UnixNano() / int64(time.Millisecond),
	}

	jsonData, err := json.MarshalIndent(mqData, "", "  ")
	if err != nil {
		return err
	}

	mq.Insertmqqt(workerCtx, string(jsonData), "Kaspa_Direct_Exchange", "Kaspa_Direct_Routing")

	workerCtx.Logger.Info(fmt.Sprintf("client authorized, address: %s", workerCtx.WalletAddr()))
	return nil
}
