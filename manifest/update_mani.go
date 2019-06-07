package manifest

import (
  "os"
  "fmt"
  "time"
  "io/ioutil"
  "net/http"
  "net/url"
  "github.com/andelf/go-curl"
  "github.com/golang/glog"
  "bytes"
  //"strconv"
  "github.com/docker/docker/api/types/container"
  "github.com/docker/docker/api/types/network"
)

// Define instruction for updater to follow
type UpdateOp int

const (
  // The component need to be updated
  COMPOP_UPDATE UpdateOp = iota
  // The component need to be deprecated
  COMPOP_DEPRECATE
)
// Component definition, including informantion like what to run and how to run
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
  Op UpdateOp `json:"op,omitempty"`
  Force bool `json:"force,omitempty"`
  Cred string `json:"cred,omitempty"`
}
// Component details for update
type UpdateManifest struct {
  Signature string `json:"signature,omitempty"`
  Digest string `json:"digest,omitempty"`
  CreatedAt time.Time `json:"created_at,omitempty"`
  Components []Component `json:"components"`
}

// names of known components
const (
  COMP_UPDATER = "updater"
)

func (self *UpdateManifest) Encode() (string, error) {
  return EncodeManifest(self)
}

func (self *UpdateManifest) Decode(data string) error {
  return DecodeMani(self, data)
}

func (self *UpdateManifest) Generate(out_path string) error {
  data, err := self.Encode()
  if err != nil {
    return err
  }

  if err := os.RemoveAll(out_path); err != nil {
    fmt.Println(err)
  }

  if err := ioutil.WriteFile(out_path, []byte(data), 0600); err != nil {
    return err
  }
  return nil
}

func (self *UpdateManifest) Post(target url.URL) error {
  data, err := self.Encode()
  if err != nil {
    return err
  }
  return self.post(target, []byte(data))
}

func (self *UpdateManifest) post(target url.URL, data []byte) error {
  data_buf := bytes.NewReader(data)
  rsp, err := http.Post(target.String(), "application/json", data_buf)
  if err != nil {
    return err
  }

  if rsp.StatusCode == http.StatusOK || rsp.StatusCode == http.StatusCreated ||
      rsp.StatusCode == http.StatusAccepted {
      return nil
  }

  return fmt.Errorf("post failed: %v", rsp.Status)
}

func FetchUpdateMani(target string) (*UpdateManifest, error) {
  url, err := url.Parse(target)
  if err != nil {
    return nil, err
  }
  glog.Info(url)
  switch url.Scheme {
    case "http", "https":
      return fetchRest(url)
    case "ftp":
      return fetchFTP(url)
  }

  return nil, fmt.Errorf("Unsupported scheme")
}

func fetchFTP(url *url.URL) (*UpdateManifest, error) {
  mani_path, err := fetchFtp(url)
  if err != nil {
    return nil, err
  }
  return DecodeFromFile(mani_path)
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

func DecodeFromFile(mani_path string) (*UpdateManifest, error) {
  mani := &UpdateManifest{}
  data, err := ioutil.ReadFile(mani_path)
  if err != nil {
    return nil, err
  }

  if err := mani.Decode(string(data)); err != nil {
    return nil, err
  }
  return mani, nil
}

func fetchRest(url *url.URL) (*UpdateManifest, error) {
  rsp, err := http.Get(url.String())
  if err != nil {
    return nil, err
  }
  defer rsp.Body.Close()

  if rsp.StatusCode != http.StatusOK {
    return nil, fmt.Errorf("fetch update manifest failed: ", rsp.Status)
  }

  buf, err := ioutil.ReadAll(rsp.Body)
  if err != nil{
    return nil, err
  }

  // data, _ := strconv.Unquote(buf)

  mani := &UpdateManifest{}
  if err := mani.Decode(string(buf)); err != nil {
    return nil, err
  }

  return mani, nil
}
