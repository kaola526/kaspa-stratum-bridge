package poolstratum

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/mattn/go-colorable"
	"github.com/onemorebsmith/poolstratum/src/chainnode/aleo"
	"github.com/onemorebsmith/poolstratum/src/gostratum"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const version = "v1.1.6"
const minBlockWaitTime = 500 * time.Millisecond

type BridgeConfig struct {
	StratumPort     string        `yaml:"stratum_port"`
	ChainType       string        `yaml:"chain_type"`
	ChainRPC        string        `yaml:"chain_rpc"`
	PromPort        string        `yaml:"prom_port"`
	PrintStats      bool          `yaml:"print_stats"`
	UseLogFile      bool          `yaml:"log_to_file"`
	HealthCheckPort string        `yaml:"health_check_port"`
	BlockWaitTime   time.Duration `yaml:"block_wait_time"`
	MinShareDiff    uint          `yaml:"min_share_diff"`
	ExtranonceSize  uint          `yaml:"extranonce_size"`
}

func configureZap(cfg BridgeConfig) (*zap.SugaredLogger, func()) {
	pe := zap.NewProductionEncoderConfig()
	pe.EncodeTime = zapcore.RFC3339TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(pe)
	consoleEncoder := zapcore.NewConsoleEncoder(pe)

	if !cfg.UseLogFile {
		return zap.New(zapcore.NewCore(consoleEncoder,
			zapcore.AddSync(colorable.NewColorableStdout()), zap.InfoLevel)).Sugar(), func() {}
	}

	// log file fun
	logFile, err := os.OpenFile("bridge.log", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		panic(err)
	}
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, zapcore.AddSync(logFile), zap.InfoLevel),
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(colorable.NewColorableStdout()), zap.InfoLevel),
	)
	return zap.New(core).Sugar(), func() { logFile.Close() }
}

func ListenAndServe(cfg BridgeConfig) error {
	logger, logCleanup := configureZap(cfg)
	defer logCleanup()

	// TODO
	if cfg.PromPort != "" {
		StartPromServer(logger, cfg.PromPort)
	}

	blockWaitTime := cfg.BlockWaitTime
	if blockWaitTime < minBlockWaitTime {
		blockWaitTime = minBlockWaitTime
	}

	// TODO
	if cfg.HealthCheckPort != "" {
		logger.Info("enabling health check on port " + cfg.HealthCheckPort)
		http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		go http.ListenAndServe(cfg.HealthCheckPort, nil)
	}

	minDiff := cfg.MinShareDiff
	if minDiff < 1 {
		minDiff = 1
	}
	extranonceSize := cfg.ExtranonceSize
	if extranonceSize > 3 {
		extranonceSize = 3
	}

	handlers := gostratum.DefaultHandlers()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var poolApi *PoolApi
	var aleoApi *aleo.AleoNode
	var err error
	poolApi, err = NewPoolAPI(cfg.ChainType, cfg.ChainRPC, blockWaitTime, logger)
	if err != nil {
		return err
	}

	if cfg.ChainType == ChainTypeKaspa {

		shareHandler := newShareHandler(poolApi.chainapi)

		clientHandler := newClientListener(logger, shareHandler, float64(minDiff), int8(extranonceSize))

		// override the submit handler with an actual useful handler
		handlers[string(gostratum.StratumMethodSubmit)] =
			func(ctx *gostratum.StratumContext, event gostratum.JsonRpcEvent) error {
				if err := shareHandler.HandleSubmit(ctx, event); err != nil {
					ctx.Logger.Sugar().Error(err) // sink error
				}
				return nil
			}

		stratumConfig := gostratum.StratumListenerConfig{
			Port:           cfg.StratumPort,
			HandlerMap:     handlers,
			StateGenerator: MiningStateGenerator,
			ClientListener: clientHandler,
			Logger:         logger.Desugar(),
		}

		poolApi.Start(ctx, func() {
			clientHandler.NewBlockAvailable(poolApi)
		})

		if cfg.PrintStats {
			go shareHandler.startStatsThread()
		}

		return gostratum.NewListener(stratumConfig).Listen(context.Background())
	} else if cfg.ChainType == ChainTypeAleo {
		aleoApi, err = aleo.NewAleoNode(cfg.ChainRPC, blockWaitTime, logger)
		if err != nil {
			return err
		}

		aleoApi.Start(ctx, func() {
			// clientHandler.NewBlockAvailable(ksApi)
		})

		// TODO
		<-ctx.Done()
		return nil
	} else {
		return fmt.Errorf("config.yaml set pool_type (%s) is error ", cfg.ChainType)
	}
}
