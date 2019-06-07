package main

import (
  "github.com/zex/container-update/manifest"
  "fmt"
  "encoding/json"
)


func gen_sample(path string) {
  meta := &manifest.UpdateManifest{
    Components: []manifest.Component {
      manifest.Component {
        Version: "0.1.2",
        Name: "Awesome Server",
        Registry: "registry.docker.co:17889",
        ImageName: "application",
        ImageTag: "0.0.1.60",
      }, manifest.Component {
        Version: "1.0.8",
        Name: "Another Awesome Server",
        Registry: "registry.docker.co:17889",
        ImageName: "glory",
        ImageTag: "0.2.3.11",
      },},}

  data, err := json.Marshal(meta)
  if err != nil {
    fmt.Println("failed to generate json: ", err)
    return
  }
  fmt.Println(string(data))
}

func main() {
  gen_sample("manifest.json")
}
