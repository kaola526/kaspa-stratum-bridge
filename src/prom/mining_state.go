package prom

import (
	"math/big"
	"sync"
	"time"

	"github.com/kaspanet/kaspad/app/appmessage"
	I "github.com/onemorebsmith/poolstratum/src/comment"
)

const maxjobs = 32

type MiningState struct {
	Jobs        map[int]*appmessage.RPCBlock
	JobLock     sync.Mutex
	JobCounter  int
	BigDiff     big.Int
	Initialized bool
	UseBigJob   bool
	ConnectTime time.Time
	StratumDiff *KaspaDiff
}

func MiningStateGenerator() any {
	return &MiningState{
		Jobs:        map[int]*appmessage.RPCBlock{},
		JobLock:     sync.Mutex{},
		ConnectTime: time.Now(),
	}
}

func GetMiningState(ctx I.WorkerClientInterface) *MiningState {
	return ctx.State().(*MiningState)
}

func (ms *MiningState) AddJob(job *appmessage.RPCBlock) int {
	ms.JobCounter++
	idx := ms.JobCounter
	ms.JobLock.Lock()
	ms.Jobs[idx%maxjobs] = job
	ms.JobLock.Unlock()
	return idx
}

func (ms *MiningState) GetJob(id int) (*appmessage.RPCBlock, bool) {
	ms.JobLock.Lock()
	job, exists := ms.Jobs[id%maxjobs]
	ms.JobLock.Unlock()
	return job, exists
}
