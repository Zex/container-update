package common

import (
  "time"
  "encoding/json"
//  "github.com/golang/glog"
)

type EventType string

const (
  EventTypeStarted EventType = "started"
  EventTypeUpdated EventType = "updated"
  EventTypeError EventType = "error"
)


type Event struct {
  Publisher
  Ty EventType `json:"type"`
  Component string `json:"component,omitempty"`
  CreatedAt time.Time `json:"created_at,omitempty"`
  Version string `json:"version,omitempty"`
  Payload string `json:"payload,omitempty"`
}

func NewEvent() *Event {
  return &Event{
    CreatedAt: time.Now(),
    Version: VERSION,
    Component: "updater",
  }
}

func NewErrEvent(payload string) *Event {
  return &Event{
    CreatedAt: time.Now(),
    Ty: EventTypeError,
    Version: VERSION,
    Component: "updater",
    Payload: payload,
  }
}

func (e *Event) Publish() error {
  data, err := json.Marshal(e)
  if err != nil { return err }
  return e.Publisher.PublishEvent(data)
}

func (e *Event) Decode(data []byte) error {
  if err := json.Unmarshal(data, e); err != nil {
    return err
  }
  return nil
}

type ByCreated []Event

func (by ByCreated) Less(i, j int) bool {
  return by[i].CreatedAt.Unix() < by[j].CreatedAt.Unix()
}
func (by ByCreated) Swap(i, j int) { by[i], by[j] = by[j], by[i] }
func (by ByCreated) Len() int { return len(by) }
