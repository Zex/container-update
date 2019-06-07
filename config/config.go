package config
/** CONFIG WITH YAML DEPRECATED

import (
  "fmt"
  "io/ioutil"
  "github.com/golang/glog"
  yaml "gopkg.in/yaml.v2"
  "github.com/zex/container-update/common"
)

type Config struct {
  BackendBase string `yaml:"backend_base"`
  ManifestPath string `yaml:"manifest_path"`
  Interval int `yaml:interval`
  PackageRoot string `yaml:"package_root"`
  Registry string `yaml:"registry"`
}

func NewConfig(path string) *Config {
  glog.Infof("%s (%v)", common.CurrentScope(), path)
  var cfg Config

  data, err := ioutil.ReadFile(path)
  if err != nil {
    panic(fmt.Sprintf("failed to read file: %v", err))
  }

  err = yaml.Unmarshal(data, &cfg)
  if err != nil {
    panic(fmt.Sprintf("failed to load config from: %v", err))
  }

  return &cfg
}
*/
