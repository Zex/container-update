package main

import (
  "fmt"
  "os"
  "github.com/zex/container-update/manifest"
  "github.com/zex/container-update/common"
)

func newSubMani() manifest.SubManifest {
  return manifest.SubManifest{
    Uri: os.Getenv("SUB_URI"),
    Cred: manifest.Credential{
      User: os.Getenv("SUB_USER"),
      Pass: os.Getenv("SUB_PASS"),
    },
    Topics: map[string]string {
      common.TopicUpdateManifest: fmt.Sprintf("%s/%s", common.TopicUpdateManifest, os.Getenv("APP_ID")),
      common.TopicEvent: fmt.Sprintf("%s/%s", common.TopicEvent, os.Getenv("APP_ID")),
      common.TopicHeartbeat: fmt.Sprintf("%s/%s", common.TopicHeartbeat, os.Getenv("APP_ID")) },}
}

func main() {
  fmt.Println(newSubMani().Encode())
}
