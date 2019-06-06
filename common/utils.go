package common

import (
  "fmt"
  "os"
  "io"
  "bytes"
  "os/exec"
  "path"
  "runtime"
  "crypto/md5"
  "net/http"
  "io/ioutil"
  "strings"
  "github.com/golang/glog"
  "path/filepath"
  "archive/zip"
  "compress/zlib"
)

func DownloadAsset(url, path string) (error) {
  glog.Infof("request %s", url)
  req, err := http.NewRequest("GET", url, nil)
  if err != nil {
    return err
  }

  cli := http.Client{}
  rsp, err := cli.Do(req)
  if err != nil {
    glog.Error("failed to fetch url", err)
    return err
  }

  if rsp == nil {
    return fmt.Errorf("empty response")
  }

  if rsp.StatusCode != http.StatusOK {
    return fmt.Errorf("unexpected status %s", rsp.StatusCode)
  }

  SaveAsset(rsp, path)
  return nil
}

func SaveAsset(rsp *http.Response, path string) (error) {
  if rsp == nil {
    return fmt.Errorf("empty response")
  }

  file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
  if err != nil {
    return err
  }
  defer file.Close()

  n_copy, err := io.Copy(file, rsp.Body)
  if err != nil {
    return err
  }

  glog.Infof("downloaded %d bytes", n_copy)
  return nil
}

func VerifyDigest(path string, target [16]byte) error {
  data, err := ioutil.ReadFile(path)
  if err != nil {
    return err
  }

  if target != md5.Sum(data) {
    return fmt.Errorf("Digest mismatch")
  }

  return nil
}

func CurrentScope() (string) {
  pc := make([]uintptr, 10)
  if 0 == runtime.Callers(2, pc) {
    return ""
  }

  frames := runtime.CallersFrames(pc)
  fr, _ := frames.Next()
  return fr.Function
}

func ExtractZip(path, dest string) error {
  r, err := zip.OpenReader(path)
  if err != nil {
    return err
  }
  defer r.Close()

  os.RemoveAll(dest)

  for _, f := range r.File {
    rc, err := f.Open()
    if err != nil {
      return err
    }
    defer rc.Close()

    out_path := filepath.Join(dest, f.Name)
    if strings.HasSuffix(f.Name, "/") {
      if err := os.MkdirAll(out_path, 0755); err != nil {
        return err
      }
      continue
    }

    if err := os.MkdirAll(filepath.Dir(out_path), 0755); err != nil {
      return err
    }

    wr, err := os.OpenFile(out_path, os.O_WRONLY|os.O_CREATE, 0600)
    if err != nil {
      return err
    }
    defer wr.Close()

    if _, err := io.Copy(wr, rc); err != nil {
      return err
    }
  }

  return nil
}

// Copy file from src to dest
func Copy(dest, src string) error {
  base := path.Dir(dest)
  if err := os.MkdirAll(base, 0755); err != nil {
    return err
  }

  src_wr, err := os.OpenFile(src, os.O_RDONLY, 0600)
  if err != nil {
    return err
  }
  defer src_wr.Close()

  dest_wr, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE, 0600)
  if err != nil {
    return err
  }
  defer dest_wr.Close()

  if _, err := io.Copy(dest_wr, src_wr); err != nil {
    return err
  }
  return nil
}

// TODO fix extraction

func NativeExtractZip(path, dest string) error {
  cmd := exec.Command("/usr/bin/unzip", "-o", "-x", path, "-d", dest)
  return RunCmd(cmd)
}

func NativeExtractTar(path, dest string) error {
  if err := os.MkdirAll(dest, 0755); err != nil {
    return err
  }

  cmd := exec.Command("/bin/tar", "xf", path, "-C", dest)
  return RunCmd(cmd)
}

func IsFile(path string) bool {
  if _, err := os.OpenFile(path, os.O_RDONLY, 0600); err != nil {
    return false
  }
  return true
}

func RunCmd(cmd *exec.Cmd) error {
  var out, eout bytes.Buffer
  cmd.Stdout = &out
  cmd.Stderr = &eout

  if err := cmd.Run(); err != nil {
    glog.Error(eout.String())
    // return err
  }
  glog.Info(out.String())
  return nil
}

func NativeMysql(db_user, db_host, db_port, pass, sql_path string) error {
  cmd := exec.Command("/usr/bin/mysql",
          fmt.Sprintf("-u%s", db_user),
          fmt.Sprintf("-h%s", db_host),
          fmt.Sprintf("-P%s", db_port),
          fmt.Sprintf("-p%s", pass),
          "-e", fmt.Sprintf("source %s", sql_path))
  return RunCmd(cmd)
}

func Compress(data []byte) ([]byte, error) {
  var buf bytes.Buffer

  w, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
  if err != nil { return nil, err }

  if _, err := w.Write(data); err != nil { return nil, err }
  w.Close()

  return buf.Bytes(), nil
}

func Decompress(data []byte) ([]byte, error) {
  r, err := zlib.NewReader(bytes.NewReader(data))
  if err != nil { return nil, err }
  defer r.Close()

  var ret bytes.Buffer
  if _, err := io.Copy(&ret, r); err != nil {
    return nil, err
  }

  return ret.Bytes(), nil
}
