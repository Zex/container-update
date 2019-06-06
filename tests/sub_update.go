package main

import (
  "os"
  "time"
  "github.com/go-stomp/stomp"
  //"github.com/jjeffery/stomp"
  "github.com/golang/glog"
  "flag"
  "encoding/json"
  "github.com/zex/container-update/manifest"
  "github.com/zex/container-update/common"
)

func Start() {
  glog.Info(common.CurrentScope())
  mq_uri := os.Getenv("CLOUD_MQ_URI")
  if mq_uri == "" {
    glog.Error("CLOUD_MQ_URI not defined")
    return
  }
  glog.Info("uri ", mq_uri)

  topic := os.Getenv("TOPIC_APPLICATION_UPDATE")
  if topic == "" {
    glog.Error("TOPIC_APPLICATION_UPDATE not defined")
    return
  }
  glog.Info("topic ", topic)

  var opts []func(*stomp.Conn) error = []func(*stomp.Conn) error {
    stomp.ConnOpt.Login(os.Getenv("MQ_LOGIN"), os.Getenv("MQ_PASS")),
    stomp.ConnOpt.Host(mq_uri),
    stomp.ConnOpt.HeartBeatError(360 * time.Second),
    //stomp.ConnOpt.HeartBeat(time.Millisecond, time.Millisecond),
  }

  conn, err := stomp.Dial("tcp", mq_uri, opts...)
  if err != nil {
    glog.Error(err)
    return
  }
  defer conn.Disconnect()
  glog.Info("Server version: ", conn.Version)

  sub, err := conn.Subscribe(topic, stomp.AckAuto)
  if err != nil {
    glog.Error(err)
    return
  }
  defer sub.Unsubscribe()

  for {
    msg := <-sub.C
    if msg.Err != nil {
      glog.Errorf("failed to recieve msg: %v", msg.Err)
      continue
    }
    on_message(msg)
  }

  // never reach
  //if err = sub.Unsubscribe(); err != nil {
  //  glog.Errorf("unsubscribe failed: %v", err)
  //}
}

func on_message(msg *stomp.Message) {
  glog.Infof("%s (%v)", common.CurrentScope(), string(msg.Body))

  var mani manifest.UpdateMeta
  if err := json.Unmarshal(msg.Body, &mani); err != nil {
    glog.Error("failed to parse json: ", err)
    return
  }

  glog.Info(mani)
  for i, comp := range mani.Components {
    glog.Infof("[%d] setup %v", i, comp)
  }
}

func main() {
  flag.Parse()
  glog.Info("Updater start")
  Start()
}
