package comment

import M "github.com/onemorebsmith/poolstratum/src/comment/model"

// 定义Worker节点接口
type WorkerClientInterface interface {
	MinerName() string
	DeviceName() string
	WalletAddr() string
	RemoteAddr() string
	Connected() bool 
	State() any
	Disconnect()
	Send(event M.JsonRpcEvent) error
}