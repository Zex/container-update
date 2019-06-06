package mqtt

import (
  "os"
  "time"
  "fmt"
  "github.com/golang/glog"
  "github.com/eclipse/paho.mqtt.golang"

  "github.com/zex/container-update/manifest"
  "github.com/zex/container-update/common"
)

var (
  Qos = byte(1)
  ConnectTimeout = "10s"
)

type MsgHandler interface {
  Handle(msg mqtt.Message)
}

type Sub struct {
  MsgHandler
  cli mqtt.Client
  opt *mqtt.ClientOptions
  mani *manifest.SubManifest
}

func NewSub(h MsgHandler) *Sub {
  ret := Sub{ MsgHandler: h}
  mani, err := manifest.LoadSubMani()
  if err != nil {
    panic(fmt.Sprintf("load subscribe manifest failed: %v", err))
  }
  ret.mani = mani
  return &ret
}

func (s *Sub) StartSub() {
  glog.Infof("%s", common.CurrentScope())
  s.run()
}

func (s *Sub) messageHandler(cli mqtt.Client, msg mqtt.Message) {
  glog.Infof("%s", common.CurrentScope())
  s.MsgHandler.Handle(msg)
}

func newClientOptions(mani *manifest.SubManifest) (*mqtt.ClientOptions) {
  glog.Infof("%s uri: %s", common.CurrentScope(), mani.Uri)
  return mqtt.NewClientOptions().AddBroker(mani.Uri).
    SetUsername(mani.Cred.User).
    SetPassword(mani.Cred.Pass)
}

func (s *Sub) SubUpdate() {
  glog.Infof("%s topic: %s", common.CurrentScope(),
    s.mani.Topics[common.TopicUpdateManifest])

  s.opt = newClientOptions(s.mani).SetOnConnectHandler(func(c mqtt.Client) {
    if token := s.cli.Subscribe(
      s.mani.Topics[common.TopicUpdateManifest], Qos, s.messageHandler);
      token.Wait() && token.Error() != nil {
      panic(token.Error())
  }})
}

func (s *Sub) SetOptions() {
  topics := map[string]byte{
    fmt.Sprintf("%s/+", common.TopicHeartbeat): Qos,
    fmt.Sprintf("%s/+", common.TopicEvent): Qos,
  }
  s.opt = newClientOptions(s.mani).SetOnConnectHandler(func(c mqtt.Client) {
    if token := s.cli.SubscribeMultiple(topics, s.messageHandler);
      token.Wait() && token.Error() != nil {
      panic(token.Error())
    }})
}

func (s *Sub) run() {
  s.cli = mqtt.NewClient(s.opt)

  if token := s.cli.Connect(); token.Wait() && token.Error() != nil {
    panic(token.Error())
  }

  c := make(chan os.Signal, 1)
  <-c
}

/** Publish update on manifest generation */
func (s *Sub) PubUpdate(mani *manifest.SubManifest, mani_bytes []byte) error {
  glog.Infof("%s topic: %s", common.CurrentScope(), mani.Topics[common.TopicUpdateManifest])

  s.cli = mqtt.NewClient(newClientOptions(mani))
  d, _ := time.ParseDuration(ConnectTimeout)
  if token := s.cli.Connect(); token.WaitTimeout(d) && token.Error() != nil {
    return token.Error()
  }
  defer s.cli.Disconnect(256)

  if token := s.cli.Publish(mani.Topics[common.TopicUpdateManifest], Qos, false, mani_bytes);
    token.Wait() && token.Error() != nil {
    return token.Error()
  }

  return nil
}

func (s *Sub) PublishHeartbeat(data []byte) error {
  glog.Infof("%s topic: %s", common.CurrentScope(), s.mani.Topics[common.TopicHeartbeat])
  if token := s.cli.Publish(s.mani.Topics[common.TopicHeartbeat], Qos, false, data);
    token.Wait() && token.Error() != nil {
    return token.Error()
  }

  return nil
}

func (s *Sub) PublishEvent(data []byte) error {
  glog.Infof("%s topic: %s", common.CurrentScope(), s.mani.Topics[common.TopicEvent])
  if token := s.cli.Publish(s.mani.Topics[common.TopicEvent], Qos, false, data);
    token.Wait() && token.Error() != nil {
    return token.Error()
  }

  return nil
}
