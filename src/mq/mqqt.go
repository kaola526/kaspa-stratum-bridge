package mq

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/rabbitmq/amqp091-go"
)

type MQKaspaData struct {
	AppName          string
}

type MQShareRecordData struct {
	AppName          string
	AppVersion       string
	RecodeType       string
	MinerName        string
	DeviceCompany    string
	DeviceType       string
	DeviceName       string
	RemoteAddr       string
	Time             int64
	Code             int
	TargetDifficulty uint64
	Params           string
}

var instance *amqp091.Channel
var mqlock sync.RWMutex

func GetChannel() *amqp091.Channel {
	mqlock.Lock()
	if instance == nil {
		instance, _ = InitMQChannel()
	}
	mqlock.Unlock()
	return instance
}

func InitMQChannel() (*amqp091.Channel, error) {
	mq_name := os.Getenv("MQ_NAME")
	if mq_name == "" {
		mq_name = "kaspapool"
	}

	mq_pwd := os.Getenv("MQ_PWD")
	if mq_pwd == "" {
		mq_pwd = "tujXiHx2ety6HRErqquML35m"
	}

	mq_ip := os.Getenv("MQ_IP")
	if mq_ip == "" {
		mq_ip = "192.168.11.115"
	}
	mq_port := os.Getenv("MQ_PORT")
	if mq_port == "" {
		mq_port = "5672"
	}
	mqport, _ := strconv.Atoi(mq_port)

	mqCh, err := amqp091.Dial(fmt.Sprintf("amqp://%s:%s@%s:%d", mq_name, mq_pwd, mq_ip, mqport))
	if err != nil {
		fmt.Printf("insertmqqt Error:%v\n", err)
		return nil, err
	}
	return mqCh.Channel()
}

func Insertmqqt(ctx context.Context, data string, exchange string, routing string) (err error) {
	ch := GetChannel()
	if ch == nil {
		fmt.Printf("mq error: not channel\n")
		return
	}

	fmt.Printf("mq1: %s\n", data)

	err = ch.PublishWithContext(
		ctx,
		exchange,
		routing,
		false,
		false,
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        []byte(data),
		},
	)
	if err != nil {
		return
	}

	return nil
}
