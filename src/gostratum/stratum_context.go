package gostratum

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	M "github.com/onemorebsmith/poolstratum/src/comment/model"
	"github.com/onemorebsmith/poolstratum/src/mq"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type WorkerContext struct {
	parentContext    context.Context
	AppName          string
	AppVersion       string
	minerName        string
	DeviceCompany    string
	DeviceType       string
	deviceName       string
	remoteAddr       string
	walletAddr       string
	TargetDifficulty int64
	Id               int32
	Logger           *zap.Logger
	connection       net.Conn
	disconnecting    bool
	onDisconnect     chan *WorkerContext
	state            any // gross, but go generics aren't mature enough this can be typed 😭
	writeLock        int32
	Extranonce       string
}

type ContextSummary struct {
	RemoteAddr string
	WalletAddr string
	DeviceName string
	MinerName  string
}

var ErrorDisconnected = fmt.Errorf("disconnecting")

func (sc *WorkerContext) Connected() bool {
	return !sc.disconnecting
}

func (sc *WorkerContext) Summary() ContextSummary {
	return ContextSummary{
		RemoteAddr: sc.remoteAddr,
		WalletAddr: sc.walletAddr,
		DeviceName: sc.deviceName,
		MinerName:  sc.minerName,
	}
}

func NewMockContext(ctx context.Context, logger *zap.Logger, state any) (*WorkerContext, *MockConnection) {
	mc := NewMockConnection()
	return &WorkerContext{
		parentContext: ctx,
		state:         state,
		remoteAddr:    "127.0.0.1",
		walletAddr:    uuid.NewString(),
		deviceName:    uuid.NewString(),
		minerName:     "mock.context",
		Logger:        logger,
		connection:    mc,
	}, mc
}

func (sc *WorkerContext) String() string {
	serialized, _ := json.Marshal(sc)
	return string(serialized)
}

func (sc *WorkerContext) Reply(response M.JsonRpcResponse) error {
	if sc.disconnecting {
		return ErrorDisconnected
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return errors.Wrap(err, "failed encoding jsonrpc response")
	}
	encoded = append(encoded, '\n')
	return sc.writeWithBackoff(encoded)
}

func (sc *WorkerContext) Send(event M.JsonRpcEvent) error {
	if sc.disconnecting {
		return ErrorDisconnected
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return errors.Wrap(err, "failed encoding jsonrpc event")
	}
	encoded = append(encoded, '\n')
	return sc.writeWithBackoff(encoded)
}

var errWriteBlocked = fmt.Errorf("error writing to socket, previous write pending")

func (sc *WorkerContext) write(data []byte) error {
	if atomic.CompareAndSwapInt32(&sc.writeLock, 0, 1) {
		defer atomic.StoreInt32(&sc.writeLock, 0)
		deadline := time.Now().Add(5 * time.Second)
		if err := sc.connection.SetWriteDeadline(deadline); err != nil {
			return errors.Wrap(err, "failed setting write deadline for connection")
		}
		_, err := sc.connection.Write(data)
		sc.checkDisconnect(err)
		return err
	}
	return errWriteBlocked
}

func (sc *WorkerContext) writeWithBackoff(data []byte) error {
	for i := 0; i < 3; i++ {
		err := sc.write(data)
		if err == nil {
			return nil
		} else if err == errWriteBlocked {
			time.Sleep(5 * time.Millisecond)
			continue
		} else {
			return err
		}
	}
	// this should virtually never happen on a 'healthy' connection. Writes
	// to the socket are actually just writing to the outgoing buffer for the
	// connection in the OS, if this blocks it's because the receiver has not
	// read from the buffer for such a length of time that the tx buffer is full
	return fmt.Errorf("failed writing to socket after 3 attempts")
}

func (sc *WorkerContext) ReplyStaleShare(id any) error {
	return sc.Reply(M.JsonRpcResponse{
		Id:     id,
		Result: nil,
		Error:  []any{21, "Job not found", nil},
	})
}
func (sc *WorkerContext) ReplyDupeShare(id any) error {
	return sc.Reply(M.JsonRpcResponse{
		Id:     id,
		Result: nil,
		Error:  []any{22, "Duplicate share submitted", nil},
	})
}

func (sc *WorkerContext) ReplyBadShare(id any) error {
	return sc.Reply(M.JsonRpcResponse{
		Id:     id,
		Result: nil,
		Error:  []any{20, "Unknown problem", nil},
	})
}

func (sc *WorkerContext) ReplyLowDiffShare(id any) error {
	return sc.Reply(M.JsonRpcResponse{
		Id:     id,
		Result: nil,
		Error:  []any{23, "Invalid difficulty", nil},
	})
}

func (sc *WorkerContext) Disconnect() {
	if !sc.disconnecting {
		mqData := mq.MQShareRecordData{
			MessageId:     uuid.New().String(),
			AppName:       sc.AppName,
			AppVersion:    sc.AppVersion,
			RecodeType:    "Logout",
			MinerName:     sc.minerName,
			DeviceCompany: sc.DeviceCompany,
			DeviceType:    sc.DeviceType,
			DeviceName:    sc.deviceName,
			RemoteAddr:    sc.remoteAddr,
			Time:          time.Now().UnixNano() / int64(time.Millisecond),
		}

		jsonData, err := json.MarshalIndent(mqData, "", "  ")
		if err == nil {
			mq.Insertmqqt(sc.parentContext, string(jsonData), "Kaspa_Direct_Exchange", "Kaspa_Direct_Routing")
		}

		sc.Logger.Info("disconnecting")
		sc.disconnecting = true
		if sc.connection != nil {
			sc.connection.Close()
		}
		sc.onDisconnect <- sc
	}
}

func (sc *WorkerContext) checkDisconnect(err error) {
	if err != nil { // actual error
		go sc.Disconnect() // potentially blocking, so async it
	}
}

// Context interface impl

func (WorkerContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (WorkerContext) Done() <-chan struct{} {
	return nil
}

func (WorkerContext) Err() error {
	return nil
}

func (d WorkerContext) Value(key any) any {
	return d.parentContext.Value(key)
}

func (d WorkerContext) MinerName() string {
	return d.minerName
}

func (d WorkerContext) DeviceName() string {
	return d.deviceName
}

func (d WorkerContext) WalletAddr() string {
	return d.walletAddr
}

func (d WorkerContext) RemoteAddr() string {
	return d.remoteAddr
}

func (d WorkerContext) State() any {
	return d.state
}
