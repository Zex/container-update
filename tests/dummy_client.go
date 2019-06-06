package main


import (
  "github.com/golang/glog"
  "io/ioutil"
  "encoding/json"
  "github.com/zex/container-update/updater"
  "github.com/zex/container-update/manifest"
  "flag"
)

func main() {
  flag.Parse()
  app := updater.NewUpdater()
  var comp manifest.Component
  data, err := ioutil.ReadFile("config/sample_manifest.json")
  if err != nil {
    glog.Errorf("faile to read manifest", err)
    return
  }

  if err := json.Unmarshal(data, &comp); err != nil {
    glog.Errorf("faile to load json", err)
    return
  }

  if err := app.SetupContainer(&comp); err != nil {
    glog.Errorf("failed to setup container: %v", err)
  }
}
