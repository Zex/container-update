package common

import (
  "fmt"
  "strings"
  "net/http"
  "net/url"
  "io/ioutil"
  "encoding/json"
  "encoding/base64"
)


type Registry struct {
  Host string
  RegAuth string
  AuthHost string
  Cli http.Client
}

func NewRegistry(auth, reg, auth_svr string) (*Registry) {
  return &Registry{reg, auth, auth_svr, http.Client{}}
}

func (r *Registry) GetUserPasswd() (string, string) {
  s, err := base64.StdEncoding.DecodeString(r.RegAuth)
  if err != nil { return "", "" }
  parts := strings.Split(string(s), ":")
  if parts == nil || len(parts) < 2 { return "", "" }
  return parts[0], parts[1]
}

func (r *Registry) ListTags(name string) ([]string, error) {
  target := url.URL{
    Scheme: "http",
    Host: r.Host,
    Path: fmt.Sprintf("/v2/%s/tags/list", name),
  }

  req, err := http.NewRequest("GET", target.String(), nil)
  if r.RegAuth != "" {
    auth, err := r.GetToken(name)
    if err != nil { return nil, err }
    req.Header.Add("Content-Type", "application/json; charset=utf-8")
    req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", auth))
  }

  rsp, err := r.Cli.Do(req)
  if err != nil {
    return nil, err
  }
  defer rsp.Body.Close()

  buf, err := ioutil.ReadAll(rsp.Body)
  if err != nil {
    return nil, err
  }

  tagsRsp := struct {
    Tags []string `json:"tags"`
  }{}

  if err := json.Unmarshal(buf, &tagsRsp); err != nil {
    return nil, err
  }

  return tagsRsp.Tags, nil
}

func (r *Registry) GetLatestTag(name string) (string, error) {
  tags, err := r.ListTags(name)
  if err != nil {
    return "", err
  }

  if tags == nil || len(tags) == 0 {
    return "", nil
  }

  SortTags(tags)
  return tags[len(tags)-1], nil
}

func (r *Registry) GetToken(name string) (string, error) {

  val := url.Values{}
  val.Add("service", r.Host)
  val.Add("scope", fmt.Sprintf("repository:%s:pull", name))

  target := url.URL {
    Scheme: "http",
    Host: r.AuthHost,
    Path: "auth",
    RawQuery: val.Encode(),
  }

  req, err := http.NewRequest("GET", target.String(), nil)
  req.Header.Add("Content-Type", "application/json; charset=utf-8")
  req.Header.Add("Authorization", fmt.Sprintf("Basic %s", r.RegAuth))

  rsp, err := r.Cli.Do(req)
  if err != nil {
    return "", err
  }
  defer rsp.Body.Close()

  buf, err := ioutil.ReadAll(rsp.Body)
  if err != nil {
    return "", err
  }

  auth_data := struct {
    Token string `json:"token"`
  }{}

  if err := json.Unmarshal(buf, &auth_data); err != nil {
    return "", err
  }
  return auth_data.Token, nil
}
