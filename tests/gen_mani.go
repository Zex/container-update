package main

import (
  "flag"
  "fmt"
  "time"
  "os"
  "net/url"
  "strconv"
  mq "github.com/zex/container-update/mqtt"
  "github.com/zex/container-update/manifest"
  "github.com/zex/container-update/common"
  "github.com/zex/container-update/tests/gen"
)

var (
  G *gen.Gen
)

func newSubMani() *manifest.SubManifest{
  return &manifest.SubManifest{
    Uri: os.Getenv("SUB_URI"),
    Cred: manifest.Credential{
      User: os.Getenv("SUB_USER"),
      Pass: os.Getenv("SUB_PASS"),
    },
    Topics: map[string]string {
      common.TopicUpdateManifest: fmt.Sprintf("%s/%s", common.TopicUpdateManifest, os.Getenv("ID")),
      common.TopicEvent: fmt.Sprintf("%s/%s", common.TopicEvent, os.Getenv("ID")),
      common.TopicHeartbeat: fmt.Sprintf("%s/%s", common.TopicHeartbeat, os.Getenv("ID")) },}
}

func gen_sub_mani() {
  data, err := newSubMani().Encode()
  if err != nil { panic(err) }
  fmt.Println(string(data))
}

func gen_asset_mani() {
  params := url.Values{}
  params.Add("ID", os.Getenv("ID"))

  url := url.URL{
      Scheme: "http",//"ftp",
      User: url.UserPassword(os.Getenv("ASSET_USER"), os.Getenv("ASSET_PASS")),
      Host: os.Getenv("ASSET_HOST"),
      Path: os.Getenv("ASSET_PATH"),
      RawQuery: params.Encode(),
  }

  mani := manifest.AssetManifest{
    Url: url.String(),
  }

  data, err := mani.Encode()
  if err != nil { panic(err) }
  fmt.Println(data)
}

func newUpdateManifest() *manifest.UpdateManifest {
  ret := &manifest.UpdateManifest{
    CreatedAt: time.Now(),}
  var image_name string

  if e, _ := strconv.ParseBool(os.Getenv("ENABLE_UPDATED")); e {
    image_name = fmt.Sprintf("updated/%s_x/%s", os.Getenv("MAJOR_VERSION"), os.Getenv("ID"))
    comp_updater := G.NewUpdaterComp(image_name, os.Getenv("APP_UPDATER_VERSION"))
    if comp_updater == nil { panic("invalid component updater") }
    ret.Components = append(ret.Components, *comp_updater)
  }

  if e, _ := strconv.ParseBool(os.Getenv("ENABLE_DB")); e {
    image_name = "mysql"
    comp_db := G.NewDbComp(image_name, os.Getenv("APP_DB_VERSION"))
    if comp_db == nil { panic("invalid component db") }
    ret.Components = append(ret.Components, *comp_db)
  }

  if e, _ := strconv.ParseBool(os.Getenv("ENABLE_APP")); e {
    image_name = fmt.Sprintf("application/%s_x", os.Getenv("MAJOR_VERSION"))
    comp_ := G.NewAppComp(image_name, os.Getenv("APP_VERSION"))
    if comp_ == nil { panic("invalid component application") }
    ret.Components = append(ret.Components, *comp_)
  }

  return ret
}

func gen_update_mani() error {
  mani := newUpdateManifest()
  data, err := mani.Encode()
  if err != nil {
    return err
  }
  fmt.Println(data)
  return nil
  /**
  q := url.Values{}
  q.Add("ID", os.Getenv("ID"))

  target := url.URL{
      Scheme: "http",
      Host: os.Getenv("ASSET_HOST"),
      Path: os.Getenv("ASSET_PATH"),
      RawQuery: q.Encode(),
  }

  if err := mani.Post(target); err != nil {
    return err
  }
  data, _ := mani.Encode()
  return publish_manifest([]byte(data))
  */
}

// publish manifest over mq
func publish_manifest(data []byte) error {
  fmt.Println("Publish manifest")
  mani := newSubMani()
  sub := mq.Sub{}
  return sub.PubUpdate(mani, data)
}

func encode(op_enc *string) {
  switch (*op_enc) {
    case "asset":
      gen_asset_mani()
    case "sub":
      gen_sub_mani()
    case "update":
      if err := gen_update_mani(); err != nil {
        panic(err)
      }
    default:
      fmt.Printf("[enc] unknown manifest type: [%v]\n", *op_enc)
    }
}

func dec_asset_mani(data *string) {
  mani := &manifest.AssetManifest{}
  if err := mani.Decode(*data); err != nil {
    fmt.Println("decode mani failed: ", err)
    return
  }
  fmt.Println(*mani)
}

func dec_sub_mani(data *string) {
  mani := &manifest.SubManifest{}
  if err := mani.Decode(*data); err != nil {
    fmt.Println("decode mani failed: ", err)
    return
  }
  fmt.Println(*mani)
}

func dec_update_mani(data *string) {
  mani := &manifest.UpdateManifest{}
  if err := mani.Decode(*data); err != nil {
    fmt.Println("decode mani failed: ", err)
    return
  }
  fmt.Println(*mani)
}

func decode(op_dec, data *string) {
  switch (*op_dec) {
    case "asset":
      dec_asset_mani(data)
    case "sub":
      dec_sub_mani(data)
    case "update":
      dec_update_mani(data)
    default:
      fmt.Printf("[dec] unknown manifest type: [%v]\n", *op_dec)
    }
}

func main() {
  var (
    op_enc = flag.String("encode", "", "Manifest type, ['asset','sub','update','latest']")
    op_dec = flag.String("decode", "", "Manifest type, ['asset','sub','update','latest']")
    op_latest = flag.Bool("latest", false, "Get latest image tag")
    data = flag.String("data", "", "Manifest data to decode")
  )

  flag.Parse()
  G = gen.NewGen()

  if len(*op_enc) > 0 {
    encode(op_enc)
  } else if (len(*op_dec)) > 0 {
    decode(op_dec, data)
  } else if *op_latest {
    fmt.Println(G.LatestTag(*gen.ImgName))
  }
}
