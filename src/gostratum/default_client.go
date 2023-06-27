package gostratum

import (
	"fmt"

	"github.com/mattn/go-colorable"
	M "github.com/onemorebsmith/poolstratum/src/comment/model"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
		string(M.StratumMethodSubscribe): HandleSubscribe,
		string(M.StratumMethodAuthorize): HandleAuthorize,
		string(M.StratumMethodSubmit):    HandleSubmit,
	}
}

func HandleSubscribe(ctx *WorkerContext, event M.JsonRpcEvent) error {
	// stub
	fmt.Printf("default_client HandleSubscribe %v", event);
	return nil
}

func HandleAuthorize(workerCtx *WorkerContext, event M.JsonRpcEvent) error {
	// stub
	fmt.Printf("default_client HandleAuthorize %v", event);
	return nil
}

func HandleSubmit(ctx *WorkerContext, event M.JsonRpcEvent) error {
	// stub
	ctx.Logger.Info("work submission")
	return nil
}

func SendExtranonce(ctx *WorkerContext) {
	
	if err := ctx.Send(M.NewEvent("", "set_extranonce", []any{ctx.Extranonce, len(ctx.Extranonce)})); err != nil {
		// should we doing anything further on failure
		ctx.Logger.Error(errors.Wrap(err, "failed to set extranonce").Error(), zap.Any("context", ctx))
	}
}
