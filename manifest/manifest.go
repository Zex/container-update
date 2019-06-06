package manifest

import (
  "os"
  "fmt"
  "time"
  "io/ioutil"
  "encoding/json"
  "encoding/base64"
  "net/http"
  "net/url"
  "github.com/andelf/go-curl"
  "github.com/golang/glog"
  "bytes"
  "strconv"
  "github.com/docker/docker/api/types/container"
  "github.com/docker/docker/api/types/network"

  "github.com/zex/container-update/common"
)

const (
  COMPOP_UPDATE = 0
  COMPOP_DEPRECATE = 1
)

type Component struct {
  Version string `json:"version"`
  Name string `json:"name"`
  Registry string `json:"registry"`
  ImageName string `json:"image_name"`
  ImageTag string `json:"image_tag"`
  ContainerConfig container.Config `json:"container_config,omitempty"`
  HostConfig container.HostConfig `json:"host_config,omitempty"`
  NetConfig network.NetworkingConfig `json:"net_config,omitempty"`
  ContainerName string `json:"container_name"`
  Op int `json:"op,omitempty"`
  Force bool `json:"force,omitempty"`
  Cred string `json:"cred,omitempty"`
}

type UpdateManifest struct {
  Signature string `json:"signature,omitempty"`
  Digest string `json:"digest,omitempty"`
  CreatedAt time.Time `json:"created_at,omitempty"`
  Components []Component `json:"components"`
}

type Credential struct {
  User string `json:"user"`
  Pass string `json:"pass"`
}

type SubManifest struct {
  Uri string `json:"uri"`
  Cred Credential `json:"cred,omitempty"`
  Queues map[string]string `json:"queues,omitempty"`
  Topics map[string]string `json:"topics,omitempty"`
}

type AssetManifest struct {
  // url.String()
  Url string `json:"url"`
}

// names of known components
const (
  COMP_UPDATER = "updater"
)

// Generate credential string with given username and password
func GenCred(user, pass string) (string, error) {
  cred := Credential{
    User: user,
    Pass: pass,
  }

  cred_json, err := json.Marshal(cred)
  if err != nil {
    return "", err
  }

  cred_str := base64.StdEncoding.EncodeToString(cred_json)
  if err != nil {
    return "", err
  }

  return cred_str, err
}

func GetCred(cred_str string) (*Credential, error) {
  var cred Credential

  cred_data, err := base64.StdEncoding.DecodeString(cred_str)
  if err != nil {
    return &cred, err
  }

  if err := json.Unmarshal(cred_data, &cred); err != nil {
    return &cred, err
  }

  return &cred, nil
}

func GenUpdateManifest(mani *UpdateManifest, out_path string) ([]byte, error) {
  mani_str, err := json.Marshal(&mani)
  if err != nil { return []byte{}, err }
  if out_path == "" { return mani_str, nil }

  if err := os.RemoveAll(out_path); err != nil {
    fmt.Println(err)
  }
  if err := ioutil.WriteFile(out_path, mani_str, 0600); err != nil {
    return []byte{}, err
  }
  return mani_str, nil
}

func EncodeAsset(mani interface{}) (string, error) {
  mani_json, err := json.Marshal(mani)
  if err != nil { return "", err }

  mani_z, err := common.Compress(mani_json)
  if err != nil { return "", err }

  mani_s := base64.StdEncoding.EncodeToString(mani_z)
  if err != nil { return "", err }

  return mani_s, err
}

func DecodeAsset(mani_str string) (*AssetManifest, error) {
  var mani AssetManifest

  mani_z, err := base64.StdEncoding.DecodeString(mani_str)
  if err != nil { return nil, err }

  mani_data, err := common.Decompress(mani_z)
  if err != nil { return nil, err }

  if err := json.Unmarshal(mani_data, &mani); err != nil {
    return nil, err
  }

  return &mani, nil
}

func DecodeSub(mani_str string) (*SubManifest, error) {
  var mani SubManifest

  mani_z, err := base64.StdEncoding.DecodeString(mani_str)
  if err != nil { return nil, err }

  mani_data, err := common.Decompress(mani_z)
  if err != nil { return nil, err }

  if err := json.Unmarshal(mani_data, &mani); err != nil {
    return nil, err
  }

  return &mani, nil
}

func PostUpdateManifest(target url.URL, data []byte) error {
  data_buf := bytes.NewReader(data)
  rsp, err := http.Post(target.String(), "application/json", data_buf)

  if err != nil {
    return err
  }
  defer rsp.Body.Close()

  buf, err := ioutil.ReadAll(rsp.Body)
  if err != nil{
    return err
  }

  uploadRsp := struct {
    Code int `json:"code"`
    Msg string `json:"msg"`
  }{}

  if err := json.Unmarshal(buf, &uploadRsp); err != nil {
    return err
  }

  if uploadRsp.Code != 0 {
    return fmt.Errorf("%v", uploadRsp)
  }

  return nil
}

func FetchMani(target string) (*UpdateManifest, error) {
  url, err := url.Parse(target)
  if err != nil {
    return nil, err
  }
  glog.Info(url)
  switch url.Scheme {
    case "http", "https":
      return fetchRest(url)
    case "ftp":
      return fetchManifest(url)
  }

  return nil, fmt.Errorf("Unsupported scheme")
}

func fetchManifest(url *url.URL) (*UpdateManifest, error) {
  mani_path, err := fetchFtp(url)
  if err != nil {
    return nil, err
  }
  return loadManifest(mani_path)
}

func fetchFtp(url *url.URL) (string, error) {
  mani_path := "/tmp/manifest.json"

  curl.GlobalInit(curl.GLOBAL_DEFAULT)
  defer curl.GlobalCleanup()

  c := curl.EasyInit()
  defer c.Cleanup()

  if err := os.RemoveAll(mani_path); err != nil {
    return "", err
  }

  wr, err := os.OpenFile(mani_path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
  if err != nil {
    return "", err
  }
  defer wr.Close()

  c.Setopt(curl.OPT_URL, url.String())
  c.Setopt(curl.OPT_WRITEDATA, wr)
  c.Setopt(curl.OPT_WRITEFUNCTION, func(ptr []byte, userdata interface{}) bool {
    fd := userdata.(*os.File)
    if _, err := fd.Write(ptr); err != nil {
      return false
    }
    return true
  })
  //c.Setopt(curl.OPT_FTP_USE_EPSV, true)
  //c.Setopt(curl.OPT_VERBOSE, true)

  if err := c.Perform(); err != nil {
    return "", err
  }
  return mani_path, nil
}

func loadManifest(mani_path string) (*UpdateManifest, error) {
  var meta UpdateManifest
  data, err := ioutil.ReadFile(mani_path)
  if err != nil {
    return nil, err
  }

  if err := json.Unmarshal(data, &meta); err != nil {
    return nil, err
  }
  return &meta, nil
}

func fetchRest(url *url.URL) (*UpdateManifest, error) {
  var meta UpdateManifest

  rsp, err := http.Get(url.String())
  if err != nil {
    glog.Error("failed to fetch manifest: ", err)
    return &meta, err
  }
  defer rsp.Body.Close()

  buf, err := ioutil.ReadAll(rsp.Body)
  if err != nil{
    glog.Error("failed to read response body: ", err)
    return &meta, err
  }

  mani_rsp := struct {
    Code int `json:"code"`
    Data string `json:"data"`
    Msg string `json:"msg"`
  }{}

  if err := json.Unmarshal(buf, &mani_rsp); err != nil {
    return nil, err
  }

  mani_rsp.Data, _ = strconv.Unquote(mani_rsp.Data)

  if err := json.Unmarshal([]byte(mani_rsp.Data), &meta); err != nil {
    return nil, err
  }
  return &meta, nil
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
