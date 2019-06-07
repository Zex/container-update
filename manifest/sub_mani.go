package manifest

import (
  "os"
  "fmt"
)

// Subscription manifest
type SubManifest struct {
  Uri string `json:"uri"`
  Cred Credential `json:"cred,omitempty"`
  Queues map[string]string `json:"queues,omitempty"`
  Topics map[string]string `json:"topics,omitempty"`
}

func (self *SubManifest) Decode(data string) error {
  return DecodeMani(self, data)
}

func DecodeSub(data string) (*SubManifest, error) {
  var mani SubManifest
  if err := mani.Decode(data); err != nil {
    return nil, err
  }
  return &mani, nil
}

func LoadSubMani() (*SubManifest, error) {
  mani_str := os.Getenv("SUB_MANIFEST")
  if mani_str == "" {
    return nil, fmt.Errorf("SUB_MANIFEST not defined")
  }

  mani, err := DecodeSub(mani_str)
  if err != nil {
    return nil, err
  }

  return mani, nil
}

func (self *SubManifest) Encode() (string, error) {
  return EncodeManifest(self)
}
