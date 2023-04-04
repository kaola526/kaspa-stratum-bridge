package gostratum

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kaspanet/kaspad/util"
	"github.com/mattn/go-colorable"
	"github.com/onemorebsmith/kaspastratum/src/mq"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type StratumMethod string

const (
	StratumMethodSubscribe  StratumMethod = "mining.subscribe"
	StratumMethodAuthorize  StratumMethod = "mining.authorize"
	StratumMethodSubmit     StratumMethod = "mining.submit"
)

func DefaultLogger() *zap.Logger {
	cfg := zap.NewDevelopmentEncoderConfig()
	cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	return zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(cfg),
		zapcore.AddSync(colorable.NewColorableStdout()),
		zapcore.DebugLevel,
	))
}

func DefaultConfig(logger *zap.Logger) StratumListenerConfig {
	return StratumListenerConfig{
		StateGenerator: func() any { return nil },
		HandlerMap:     DefaultHandlers(),
		Port:           ":5555",
		Logger:         logger,
	}
}

func DefaultHandlers() StratumHandlerMap {
	return StratumHandlerMap{
		string(StratumMethodSubscribe):  HandleSubscribe,
		string(StratumMethodAuthorize):  HandleAuthorize,
		string(StratumMethodSubmit):     HandleSubmit,
	}
}

func HandleAuthorize(ctx *StratumContext, event JsonRpcEvent) error {
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
	address, err := CleanWallet("kaspa:qzn4fltcsh30n22f6zszvuy9pkzjnmz97dcvm740wd5l98dqw94q6s820ggvg")
	if err != nil {
		return fmt.Errorf("invalid wallet format %s: %w", address, err)
	}

	ctx.WalletAddr = address
	ctx.MinerName = minername
	ctx.DeviceName = devicename
	ctx.Logger = ctx.Logger.With(zap.String("worker", ctx.DeviceName), zap.String("addr", ctx.WalletAddr))

	if err := ctx.Reply(NewResponse(event, true, nil)); err != nil {
		return errors.Wrap(err, "failed to send response to authorize")
	}
	if ctx.Extranonce != "" {
		SendExtranonce(ctx)
	}

	mqData := mq.MQShareRecordData{
		AppName:          ctx.AppName,
		AppVersion:       ctx.AppVersion,
		RecodeType:       "Login",
		MinerName:        ctx.MinerName,
		DeviceCompany:    ctx.DeviceCompany,
		DeviceType:       ctx.DeviceType,
		DeviceName:       ctx.DeviceName,
		RemoteAddr:       ctx.RemoteAddr,
		Time:             time.Now().UnixNano() / int64(time.Millisecond),
	}

	jsonData, err := json.MarshalIndent(mqData, "", "  ")
	if err != nil {
		return err
	}
	
	ctx.Logger.Info(fmt.Sprintf("mq: %s", jsonData))
	mq.Insertmqqt(ctx, string(jsonData), "Kaspa_Direct_Exchange", "Kaspa_Direct_Routing")
	
	ctx.Logger.Info(fmt.Sprintf("client authorized, address: %s", ctx.WalletAddr))
	return nil
}

func HandleSubscribe(ctx *StratumContext, event JsonRpcEvent) error {
	if err := ctx.Reply(NewResponse(event,
		[]any{true, "kaspa/1.0.0"}, nil)); err != nil {
		return errors.Wrap(err, "failed to send response to subscribe")
	}
	if len(event.Params) > 0 {
		app, ok := event.Params[0].(string)
		if ok {
			ctx.AppName = app
		}
	}
	if len(event.Params) > 1 {
		version, ok := event.Params[1].(string)
		if ok {
			ctx.AppVersion = version
		}
	}
	if len(event.Params) > 2 {
		deviceCompany, ok := event.Params[2].(string)
		if ok {
			ctx.DeviceCompany = deviceCompany
		}
	}
	if len(event.Params) > 3 {
		deviceType, ok := event.Params[3].(string)
		if ok {
			ctx.DeviceType = deviceType
		}
	}
	ctx.Logger.Info("client subscribed ", zap.Any("context", ctx))
	return nil
}

func HandleSubmit(ctx *StratumContext, event JsonRpcEvent) error {
	// stub
	ctx.Logger.Info("work submission")
	return nil
}

func SendExtranonce(ctx *StratumContext) {
	if err := ctx.Send(NewEvent("", "set_extranonce", []any{ctx.Extranonce, len(ctx.Extranonce)})); err != nil {
		// should we doing anything further on failure
		ctx.Logger.Error(errors.Wrap(err, "failed to set extranonce").Error(), zap.Any("context", ctx))
	}
}

var walletRegex = regexp.MustCompile("kaspa:[a-z0-9]+")

func CleanWallet(in string) (string, error) {
	_, err := util.DecodeAddress(in, util.Bech32PrefixKaspa)
	if err == nil {
		return in, nil // good to go
	}
	if !strings.HasPrefix(in, "kaspa:") {
		return CleanWallet("kaspa:" + in)
	}

	// has kaspa: prefix but other weirdness somewhere
	if walletRegex.MatchString(in) {
		return in[0:67], nil
	}
	return "", errors.New("unable to coerce wallet to valid kaspa address")
}
