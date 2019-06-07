package manifest

import (
  "encoding/base64"
  "encoding/json"
  "github.com/zex/container-update/common"
)

func EncodeManifest(mani interface{}) (string, error) {
  mani_json, err := json.Marshal(mani)
  if err != nil { return "", err }

  mani_z, err := common.Compress(mani_json)
  if err != nil { return "", err }

  mani_s := base64.StdEncoding.EncodeToString(mani_z)
  if err != nil { return "", err }

  return mani_s, err
}

func DecodeMani(mani interface{}, data string) error {
  mani_z, err := base64.StdEncoding.DecodeString(data)
  if err != nil { return err }

  mani_data, err := common.Decompress(mani_z)
  if err != nil { return err }

  if err := json.Unmarshal(mani_data, mani); err != nil {
    return err
  }
  return nil
}
