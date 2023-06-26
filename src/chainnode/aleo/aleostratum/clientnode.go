package aleostratum

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"

	M "github.com/onemorebsmith/poolstratum/src/comment/model"
	"github.com/pkg/errors"
)

const (
	Version   = 0    // 协议版本号
	RequestId = 2    // 请求 ID
	UniqueID  = 1234 // 唯一标识符，可根据实际情况调整
)

var ErrorDisconnected = fmt.Errorf("aleo chain node disconnecting")

const (
	StratumMethodSubscribe        M.StratumMethod = "m.ss"
	StratumMethodAuthorize        M.StratumMethod = "m.a"
	StratumMethodNotify           M.StratumMethod = "m.n"
	StratumMethodLocalSpeed       M.StratumMethod = "m.ls"
	StratumMethodSubmit           M.StratumMethod = "m.s"
	StratumMethodResponse         M.StratumMethod = "m.r"
	StratumMethodPuzzleResponse   M.StratumMethod = "m.pr"
	StratumMethodDirectDisconnect M.StratumMethod = "m.dd"
)

// type JsonRpcEvent struct {
// 	Id      any           `json:"id"` // id can be nil, a string, or an int 🙄
// 	Version string        `json:"jsonrpc"`
// 	Method  StratumMethod `json:"method"`
// 	Params  []any         `json:"params"`
// }

// type JsonRpcResponse struct {
// 	Id     any   `json:"id"`
// 	Result any   `json:"result"`
// 	Error  []any `json:"error"`
// }

// 链接池的配置信息
type PoolConfig struct {
	address    string
	channelid  string
	minername  string
	devicename string
}

// Stratum 协议客户端
type AleoStratumClient struct {
	Config    PoolConfig
	conn      net.Conn
	connected bool
	writeLock int32
	LastWork  *M.JsonRpcEvent
}

func NewResponse(event M.JsonRpcEvent, results any, err []any) M.JsonRpcResponse {
	return M.JsonRpcResponse{
		Id:     event.Id,
		Result: results,
		Error:  err,
	}
}

func CreateStratumClient(address string, channelid string, minername string, devicename string) *AleoStratumClient {
	return &AleoStratumClient{
		Config: PoolConfig{address, channelid, minername, devicename},
	}
}

// 注册
func (sc *AleoStratumClient) Subscribe() error {
	var err error
	fmt.Printf("step1 000\n")
	sc.conn, err = net.Dial("tcp", sc.Config.address)
	if err != nil {
		return err
	}

	sc.connected = true
	fmt.Printf("step1 111\n")
	// 渠道名，软件与版本
	jobParams := []interface{}{sc.Config.channelid, "aleopool/0.0.1", nil}
	// err = sc.writeMessage(Version, StratumMethodSubscribe, params)
	// if err != nil {
	// 	return err
	// }

	if err := sc.Send(M.JsonRpcEvent{
		Version: "2.0",
		Method:  StratumMethodSubscribe,
		Id:      0,
		Params:  jobParams,
	}); err != nil {
		fmt.Printf("step1 {err}\n")
		return err
	}

	fmt.Printf("step1 222\n")
	// 接收响应信息，包括协议版本号、唯一标识符和响应状态
	err = sc.readResponse(func(line string) error {
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("step1. subscribe ok.\n")

	return nil
}

func (sc *AleoStratumClient) Authorize() error {
	var err error

	// 发送挖矿请求，包括工作区块头信息和唯一标识符等信息
	jobParams := []interface{}{sc.Config.minername, sc.Config.devicename, nil}

	if err := sc.Send(M.JsonRpcEvent{
		Version: "2.0",
		Method:  StratumMethodAuthorize,
		Id:      0,
		Params:  jobParams,
	}); err != nil {
		fmt.Printf("step2 {err}\n")
		return err
	}

	// 接收响应信息，包括工作描述符和目标难度等信息
	err = sc.readResponse(func(line string) error {
		fmt.Println("step2 Authorize readResponse ", line)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("step2 authorize ok.\n")

	return nil
}

func (sc *AleoStratumClient) Listen(cb LineCallback) error {
	for {
		buffer := make([]byte, 4096)
		_, err := sc.conn.Read(buffer)
		if err != nil && err != io.EOF {
			fmt.Print("read err ", err, "\n")
			return errors.Wrapf(err, "error reading from connection")
		}
		buffer = bytes.ReplaceAll(buffer, []byte("\x00"), nil)
		scanner := bufio.NewScanner(strings.NewReader(string(buffer)))
		for scanner.Scan() {
			if err := cb(scanner.Text()); err != nil {
				return err
			}
		}
		time.Sleep(time.Millisecond * 10)
	}
}

type LineCallback func(line string) error

// 读取 JSON-RPC 响应消息
func (sc *AleoStratumClient) readResponse(cb LineCallback) error {
	// deadline := time.Now().Add(5 * time.Second).UTC()
	// if err := sc.conn.SetReadDeadline(deadline); err != nil {
	// 	return err
	// }

	buffer := make([]byte, 4096)
	_, err := sc.conn.Read(buffer)
	if err != nil && err != io.EOF {
		fmt.Print("read err ", err, "\n")
		return errors.Wrapf(err, "error reading from connection")
	}
	buffer = bytes.ReplaceAll(buffer, []byte("\x00"), nil)
	scanner := bufio.NewScanner(strings.NewReader(string(buffer)))
	for scanner.Scan() {
		if err := cb(scanner.Text()); err != nil {
			return err
		}
	}
	return nil
}

func (sc *AleoStratumClient) Reply(response M.JsonRpcResponse) error {
	if !sc.connected {
		return ErrorDisconnected
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return errors.Wrap(err, "failed encoding jsonrpc response")
	}
	encoded = append(encoded, '\n')
	return sc.writeWithBackoff(encoded)
}

func (sc *AleoStratumClient) Send(event M.JsonRpcEvent) error {
	if !sc.connected {
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

func (sc *AleoStratumClient) writeWithBackoff(data []byte) error {
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

func (sc *AleoStratumClient) write(data []byte) error {
	if atomic.CompareAndSwapInt32(&sc.writeLock, 0, 1) {
		defer atomic.StoreInt32(&sc.writeLock, 0)
		deadline := time.Now().Add(5 * time.Second)
		if err := sc.conn.SetWriteDeadline(deadline); err != nil {
			return errors.Wrap(err, "failed setting write deadline for connection")
		}
		_, err := sc.conn.Write(data)
		sc.checkDisconnect(err)
		return err
	}
	return errWriteBlocked
}

func (sc *AleoStratumClient) checkDisconnect(err error) {
	if err != nil { // actual error
		go sc.Disconnect() // potentially blocking, so async it
	}
}

func (sc *AleoStratumClient) Disconnect() {
	if sc.connected {
		// TODO 处理断开逻辑
	}
}

// func (sc *StratumClient) Close() error {
// 	return fmt.Errorf("aleo not Close")
// }

// func (sc *StratumClient) Reconnect() error {
// 	return fmt.Errorf("aleo not Reconnect")
// }

// func (sc *StratumClient) GetBlockDAGInfo() (*appmessage.GetBlockDAGInfoResponseMessage, error) {
// 	return nil, fmt.Errorf("aleo not GetBlockDAGInfo")
// }
// func (sc *StratumClient) EstimateNetworkHashesPerSecond(startHash string, windowSize uint32) (*appmessage.EstimateNetworkHashesPerSecondResponseMessage, error) {
// 	return nil, fmt.Errorf("aleo not EstimateNetworkHashesPerSecond")
// }
// func (sc *StratumClient) GetInfo() (*appmessage.GetInfoResponseMessage, error) {
// 	return nil, fmt.Errorf("aleo not GetInfo")
// }
// func (sc *StratumClient) RegisterForNewBlockTemplateNotifications(onNewBlockTemplate func(notification *appmessage.NewBlockTemplateNotificationMessage)) error {
// 	return fmt.Errorf("aleo not RegisterForNewBlockTemplateNotifications")
// }
// func (sc *StratumClient) GetBlockTemplate(miningAddress, extraData string) (*appmessage.GetBlockTemplateResponseMessage, error) {
// 	return nil, fmt.Errorf("aleo not GetBlockTemplate")
// }
// func (sc *StratumClient) GetBalancesByAddresses(addresses []string) (*appmessage.GetBalancesByAddressesResponseMessage, error) {
// 	return nil, fmt.Errorf("aleo not GetBalancesByAddresses")
// }
// func (sc *StratumClient) SubmitBlock(block *externalapi.DomainBlock) (appmessage.RejectReason, error) {
// 	return appmessage.RejectReasonNone, fmt.Errorf("aleo not SubmitBlock")
// }
