package manifest

import (
  "encoding/base64"
  "github.com/zex/container-update/manifest/pb"
  "github.com/golang/protobuf/proto"
)

// Credential required to get update materials
type Credential struct {
  User string `json:"user"`
  Pass string `json:"pass"`
}
// Generate credential string with given username and password
func GenCred(user, pass string) (string, error) {
  cred := &pb.Credential{
    User: user,
    Pass: pass,
  }

  cred_data, err := proto.Marshal(cred)
  if err != nil {
    return "", err
  }

  cred_str := base64.StdEncoding.EncodeToString(cred_data)
  if err != nil {
    return "", err
  }

  return cred_str, err
}

func GetCred(cred_str string) (*Credential, error) {
  var cred pb.Credential

  cred_data, err := base64.StdEncoding.DecodeString(cred_str)
  if err != nil {
    return nil, err
  }

  if err := proto.Unmarshal(cred_data, &cred); err != nil {
    return nil, err
  }

  return &Credential{User: cred.User, Pass: cred.Pass}, nil
}
