package sched

import (
  "os"
  "time"
  "fmt"
  "github.com/golang/glog"
  "github.com/zex/container-update/common"
)

var (
//  dur = readDuration()
)

const (
  SCHED_DURATION_DEFAULT = "24h"
)

type TimeoutHandler interface {
  RunOnce()
}

type Sched struct {
  TimeoutHandler
  dur time.Duration
}

func (s *Sched) StartSched() {
  glog.Infof("%s", common.CurrentScope())
  time.AfterFunc(s.dur, s.sched_timeout)
  s.run()
}

func (s *Sched) run() {
  c := make(chan os.Signal, 1)
  <-c
}

func readDuration() (time.Duration) {
  dur_s := os.Getenv("SCHED_DURATION")
  if dur_s == "" {
    dur_s = SCHED_DURATION_DEFAULT
  }

  dur, err := time.ParseDuration(dur_s)
  if err != nil {
    panic(fmt.Sprintf("invalid duration: %v", err))
  }

  return dur
}

func (s *Sched) sched_timeout() {
  glog.Infof("%s", common.CurrentScope())
  s.TimeoutHandler.RunOnce()
  time.AfterFunc(s.dur, s.sched_timeout)
}

func NewSched(h TimeoutHandler) *Sched {
  return &Sched{
    TimeoutHandler: h,
    dur: readDuration(),
  }
}
