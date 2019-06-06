package stomp

import (
  "time"
  "encoding/json"
  "github.com/golang/glog"
  "github.com/go-stomp/stomp"
  "github.com/zex/container-update/common"
  "github.com/zex/container-update/manifest"
)

type MsgHandler interface {
  Handle(msg *stomp.Message)
}

type Sub struct {
  MsgHandler
  sub *stomp.Subscription
}

func NewSub(h MsgHandler) Sub {
  return Sub{ MsgHandler: h }
}

func (s *Sub) subUpdate(mani *manifest.SubManifest) {
  glog.Infof("%s", common.CurrentScope())

  var opts []func(*stomp.Conn) error = []func(*stomp.Conn) error {
    stomp.ConnOpt.Login(mani.Cred.User, mani.Cred.Pass),
    stomp.ConnOpt.Host(mani.Uri),
    stomp.ConnOpt.HeartBeatError(360 * time.Second),
    //stomp.ConnOpt.HeartBeat(time.Millisecond, time.Millisecond),
  }

  conn, err := stomp.Dial("tcp", mani.Uri, opts...)
  if err != nil {
    glog.Error(err)
    return
  }
  glog.Info("Server version: ", conn.Version)

  s.sub, err = conn.Subscribe(mani.Queues[common.TopicUpdateManifest], stomp.AckAuto)
  if err != nil {
    glog.Error(err)
    return
  }

  defer conn.Disconnect()
  defer s.sub.Unsubscribe()
  s.run()
}

func (s *Sub) run() {
  for {
    msg := <-s.sub.C
    if msg.Err != nil {
      glog.Errorf("failed to recieve msg: %v", msg.Err)
      continue
    }
    s.on_message(msg)
  }
}

// subscribe to update manifest
func (s *Sub) StartSub() {
  glog.Infof("%s", common.CurrentScope())
  mani, err := manifest.LoadSubMani()
  if err != nil {
    glog.Error("load subscribe manifest failed: ", err)
    return
  }

  s.subUpdate(mani)
}

func (s *Sub) on_message(msg *stomp.Message) {
  glog.Infof("%s (%v)", common.CurrentScope(), string(msg.Body))

  s.MsgHandler.Handle(msg)
}
