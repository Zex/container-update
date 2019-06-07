package manifest

// Information to get update manifest
type AssetManifest struct {
  // url.String()
  Url string `json:"url"`
}

func (self *AssetManifest) Decode(data string) error {
  return DecodeMani(self, data)
}

func (self *AssetManifest) Encode() (string, error) {
  return EncodeManifest(self)
}

func DecodeAsset(data string) (*AssetManifest, error) {
  var mani AssetManifest
  if err := mani.Decode(data); err != nil {
    return nil, err
  }
  return &mani, nil
}
