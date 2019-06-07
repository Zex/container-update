package updater

import (
  "os"
  "fmt"
  "sync"
  "time"
  "io/ioutil"
  "os/exec"
  "path/filepath"
  "github.com/golang/glog"
  "github.com/docker/docker/api/types"

  mq "github.com/zex/container-update/mqtt"
  "github.com/zex/container-update/manifest"
  "github.com/zex/container-update/common"
)

var (
  POST_OP_MARKER = "/opt/.updater_post_op"
  UPDATER_SERVICE = os.Getenv("UPDATER_SERVICE")
  UPDATER_IN_CONTAINER = os.Getenv("UPDATER_IN_CONTAINER")
  // paths on host
  UPDATER_ROOT = os.Getenv("UPDATER_ROOT")
)

type DockerUpdater struct {
  setup_mutex *sync.Mutex
  adapt IDocker
  sub *mq.Sub
}

func NewDockerUpdater(sub *mq.Sub) *DockerUpdater {
  return &DockerUpdater {
    setup_mutex: &sync.Mutex{},
    adapt: NewDockerAdapter(),
    sub: sub,
  }
}

func (self *DockerUpdater) SetupComponents(mani *manifest.UpdateManifest) {
  glog.Infof("%s", common.CurrentScope())
  if err := detectEnv(); err != nil {
    glog.Error(err)
    return
  }

  self.setup_mutex.Lock()

  for i, comp := range mani.Components {
    glog.Infof("[%d] setup %v", i, comp)
    emsg := ""

    switch comp.Name {
    case manifest.COMP_UPDATER:
      if self.onUpdaterPostOp() {
        os.RemoveAll(POST_OP_MARKER)
        if err := self.adapt.SetupContainer(&comp, true); err != nil {
            // self.PostSetupDB); err != nil {
          emsg = fmt.Sprintf("failed to setup container: %v", err)
          glog.Error(emsg)
        }
      } else if !self.adapt.NeedUpdate(&comp) {
        continue
      } else {
        if err := self.setUpdaterPostOp(); err != nil {
          emsg := fmt.Sprintf("failed to set updater post op: %v", err)
          glog.Error(emsg)
        }

        if err := self.adapt.SetupContainer(&comp, false,
            self.PostSetupUpdater,
            // updater exits after deploy
            self.PostSetupUpdaterDeploy); err != nil {
          emsg := fmt.Sprintf("failed to setup container: %v", err)
          glog.Error(emsg)
        }
      }
    default:
      if err := self.adapt.SetupContainer(&comp, false); err != nil {
        emsg = fmt.Sprintf("failed to setup container: %v", err)
        glog.Error(emsg)
      }
    }

    if len(emsg) != 0 {
      self.pubError(emsg)
    }
  }

  if err := self.heartbeatUpdate(); err != nil {
    glog.Errorf("heartbeat failed: %v", err)
  }
  self.setup_mutex.Unlock()
}

// Post Operation callback
func (self *DockerUpdater) PostSetupUpdaterDeploy(comp *manifest.Component) error {
  glog.Infof("%s", common.CurrentScope())

  // updater must not disable/stop itself
  cmd := exec.Command("/bin/systemctl", "daemon-reload")
  if err := common.RunCmd(cmd); err != nil {
    return err
  }

  cmd = exec.Command("/bin/systemctl", "enable", UPDATER_SERVICE)
  if err := common.RunCmd(cmd); err != nil {
    return err
  }
  /**
  cmd = exec.Command("/bin/systemctl", "restart", UPDATER_SERVICE)
  if err := common.RunCmd(cmd); err != nil {
    return err
  }
  */
  self.setup_mutex.Unlock()
  os.Exit(1)

  return nil
}

// Post Operation callback
func (self *DockerUpdater) PostSetupUpdater(comp *manifest.Component) error {
  glog.Infof("%s", common.CurrentScope())

  cont, err := self.adapt.GetContainersByName(comp.ContainerName)
  if err != nil { return err }

  if err := self.extractUpdaterContent(cont); err != nil {
    return err
  }

  if err := self.deployUpdater(); err != nil {
    return err
  }

  return nil
}

func (self *DockerUpdater) deployUpdater() error {
  if err := os.Chmod(filepath.Join(UPDATER_ROOT, "scripts/updater.sh"), 0755); err != nil {
    return err
  }

  if err := common.Copy(filepath.Join(UPDATER_ROOT, fmt.Sprintf("config/%s.service", UPDATER_SERVICE)),
    fmt.Sprintf("/etc/systemd/system/%s.service", UPDATER_SERVICE)); err != nil{
    return err
  }

  updater_path := filepath.Join(UPDATER_ROOT, "build/container-update/updated")
  if err := os.Chmod(updater_path, 0755); err != nil {
    return err
  }
  return nil
}

func (self *DockerUpdater) extractUpdaterContent(cont *types.Container) (error) {
  glog.Infof("%s", common.CurrentScope())

  src_path := UPDATER_IN_CONTAINER
  dest_path := filepath.Join("/tmp", filepath.Base(src_path))

  if err := self.adapt.CopyFromContainer(cont, src_path, dest_path); err != nil {
    return err
  }
  defer os.RemoveAll(dest_path)

  os.RemoveAll(UPDATER_ROOT)

  glog.Infof("%s => %s", dest_path, UPDATER_ROOT)
  if err := common.NativeExtractZip(dest_path, UPDATER_ROOT); err != nil {
    // TODO
    glog.Errorf("failed to extract updater: %v", err)
  }

  return nil
}

func (self *DockerUpdater) onUpdaterPostOp() bool {
  glog.Infof("%s", common.CurrentScope())
  fd, err := os.OpenFile(POST_OP_MARKER, os.O_RDONLY, 0600)
  if err == nil {
    defer fd.Close()
    glog.Infof("updater post op")
    return true
  }
  glog.Error(err)
  return false
}

func (self *DockerUpdater) setUpdaterPostOp() error {
  glog.Infof("%s", common.CurrentScope())
  now, err := time.Now().MarshalText()
  if err != nil {
    return err
  }
  return ioutil.WriteFile(POST_OP_MARKER, now, 0600)
}

func (self *DockerUpdater) heartbeatUpdate() error {
  glog.Infof("%s", common.CurrentScope())
  var err error

  hb := common.NewUpdaterHeartbeat()
  hb.Publisher = self.sub

  if hb.Containers, err = self.adapt.ListContainers(); err != nil {
    hb.Error = fmt.Sprintf("failed to list containers", err)
    return hb.Publish()
  }

  if hb.Images, err = self.adapt.ListImages(); err != nil {
    hb.Error = fmt.Sprintf("failed to list images", err)
    return hb.Publish()
  }

  return hb.Publish()
}

func (self *DockerUpdater) pubError(e string) {
  ev := common.NewErrEvent(e)
  ev.Publisher = self.sub
  ev.Publish()
}
