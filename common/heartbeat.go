package common

import (
  "bytes"
  "net/http"
  "net/url"
  "time"
  "os"
  "encoding/json"
  "fmt"
  "github.com/golang/glog"
  "github.com/docker/docker/api/types"
)

const (
  API_HEARTBEAT = "/heartbeat"
)

type Heartbeat struct {
  Publisher
  Component string `json:"component,omitempty"`
  CreatedAt time.Time `json:"created_at,omitempty"`
  Containers []types.Container `json:"containers,omitempty"`
  Images []types.ImageSummary `json:"images,omitempty"`
  Version string `json:"version,omitempty"`
  Error string `json:"error,omitempty"`
}

type HeartbeatUI struct{
  AppId string `json:"app_id,omitempty"`
  CreatedAt time.Time `json:"created_at,omitempty"`
  Containers []types.Container `json:"containers,omitempty"`
}

func NewUpdaterHeartbeat() *Heartbeat {
  return &Heartbeat{
    Component: "updater",
    CreatedAt: time.Now(),
    Version: VERSION,
  }
}

func (hb *Heartbeat) Post() error {
  target, err := url.Parse(os.Getenv("BACKEND_BASE"))
  if err != nil { return err }

  target.Path = API_HEARTBEAT
  glog.Infof("post heartbeat %s", target.String())

  data, err := json.Marshal(hb)
  if err != nil { return err }

  rd := bytes.NewReader(data)
  rsp, err := http.Post(target.String(), "application/json", rd)

  if rsp == nil {
    return fmt.Errorf("empty response")
  }

  if rsp.StatusCode != http.StatusOK {
    return fmt.Errorf("unexpected status %v", rsp)
  }
  return nil
}

func (hb *Heartbeat) Publish() error {
  data, err := json.Marshal(hb)
  if err != nil { return err }
  return hb.Publisher.PublishHeartbeat(data)
}

func (hb *Heartbeat) Decode(data []byte) error {
  if err := json.Unmarshal(data, hb); err != nil {
    return err
  }
  return nil
}

func BuildHeartbeatUI(hbs []HeartbeatUI) ([]byte, error) {
  status := &struct{
    Status []HeartbeatUI `json:"status,omitempty"`
  }{ Status: hbs }

  data, err := json.Marshal(status)
  if err != nil {
    glog.Error("marshall failed: ", err)
    return nil, err
  }
  return data, nil
}
