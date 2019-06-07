package main

import (
  "github.com/golang/glog"
  "flag"
  up "github.com/zex/container-update/updater"
  "github.com/zex/container-update/common"
)

func main() {
  flag.Parse()
  glog.Infof("Updater %s", common.VERSION)
  app := up.NewDaemon()
  app.Start()
}
