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
  "github.com/zex/container-update/test/gen"
)

var (
  G *gen.Gen
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

func gen_sub_mani() {
  mani_str, err := manifest.EncodeAsset(newSubMani())
  if err != nil { panic(err) }
  fmt.Println(string(mani_str))
}

func gen_asset_mani() {
  params := url.Values{}
  params.Add("appId", os.Getenv("APP_ID"))

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

  mani_str, err := manifest.EncodeAsset(&mani)
  if err != nil { panic(err) }
  fmt.Println(string(mani_str))
}

func newUpdateManifest() (*manifest.UpdateManifest) {
  ret := &manifest.UpdateManifest{
    CreatedAt: time.Now(),}
  var image_name string

  if e, _ := strconv.ParseBool(os.Getenv("ENABLE_UPDATER")); e {
    image_name = fmt.Sprintf("updater/%s_x/%s", os.Getenv("MAJOR_VERSION"), os.Getenv("APP_ID"))
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

  if e, _ := strconv.ParseBool(os.Getenv("ENABLE_MQ")); e {
    image_name = "activemq"
    comp_mq := G.NewMqComp(image_name, os.Getenv("APP_MQ_VERSION"))
    if comp_mq == nil { panic("invalid component mq") }
    ret.Components = append(ret.Components, *comp_mq)
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
  mani_path := fmt.Sprintf("config/manifest-%s.json", G.Reg.Host)
  mani_bytes, err := manifest.GenUpdateManifest(
    newUpdateManifest(), mani_path)
  if err != nil { return err }

  if err := post_manifest(mani_bytes); err != nil {
    return err
  }
  return publish_manifest(mani_bytes)
}

// publish manifest over mq
func publish_manifest(mani_bytes []byte) error {
  fmt.Println("Publish manifest")
  mani := newSubMani()
  sub := mq.Sub{}
  return sub.PubUpdate(&mani, mani_bytes)
}

// post manifest to backend
func post_manifest(mani_bytes []byte) error {
  q := url.Values{}
  q.Add("appId", os.Getenv("APP_ID"))

  target := url.URL{
      Scheme: "http",
      Host: os.Getenv("ASSET_HOST"),
      Path: os.Getenv("ASSET_PATH"),
      RawQuery: q.Encode(),
  }

//  data.Add("content", string(mani_bytes))
  fmt.Println("upload update manifest: ", target.String())
  return manifest.PostUpdateManifest(target, mani_bytes)
}

func main() {
  var (
    op = flag.String("op", "", "Manifest type, ['asset','sub','update','latest']")
  )

  flag.Parse()
  G = gen.NewGen()

  switch (*op) {
    case "asset":
      gen_asset_mani()
    case "sub":
      gen_sub_mani()
    case "update":
      fmt.Println("update manifest")
      if err := gen_update_mani(); err != nil {
        panic(err)
      }
    case "latest":
      fmt.Println(G.LatestTag(*gen.ImgName))
    default:
      fmt.Printf("unknown manifest type: [%s]\n", op)
    }
}

