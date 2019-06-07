package updater

import (
  "fmt"
  "os"
  "io"
  "context"
  "encoding/json"
  "encoding/base64"
  "github.com/golang/glog"
	"github.com/docker/docker/pkg/parsers/operatingsystem"
  "github.com/docker/docker/api/types"
  "github.com/docker/docker/api/types/filters"
  //"github.com/docker/docker/libcontainerd"
	"github.com/containerd/containerd"
  docker "github.com/docker/docker/client"

  "github.com/zex/container-update/manifest"
  "github.com/zex/container-update/common"
)

func detectEnv() error {
  if in_cont, err := operatingsystem.IsContainerized(); err != nil {
    glog.Infof("cannot detect whether we are containerized")
  } else if in_cont {
    return fmt.Errorf("updater should not be running in container")
  }
  return nil
}

type DockerAdapter struct {
  ctx context.Context
  cli *docker.Client
}

func NewDockerAdapter() *DockerAdapter {
  var err error
  ret := &DockerAdapter{
    ctx: context.Background(),
  }

  if ret.cli, err = docker.NewEnvClient(); err != nil {
    glog.Fatal(err)
  }
  return ret
}

func containerBackupName(name string) string {
  return fmt.Sprintf("%s-prev", name)
}

func (self *DockerAdapter) SetupContainer(
  comp *manifest.Component, post_only bool, funcs... PostSetupFn) error {
  glog.Infof("%s (post_only=%d)", common.CurrentScope(), post_only)

  if !self.NeedUpdate(comp) {
    if post_only { self.performPostOp(comp, funcs...) }
    glog.Infof("%s: update not needed", comp.Name)
    return nil
  }

  if err := self.FetchImage(comp); err != nil {
    return fmt.Errorf("failed to pull image: %v", err)
  }

  prev, err := self.GetContainersByName(comp.ContainerName)
  if err != nil {
    glog.Infof("faile to list container: %v", err)
  }

  if prev != nil {
    if err := self.CleanupContainer(prev); err != nil {
      glog.Errorf("failed to cleanup previous container: %v", err)
    }
  }

  /**
  prev, err := self.BackupContainer(comp)
  if err != nil {
    return err
  }
  */

  if err := self.StartContainer(comp); err != nil {
    return fmt.Errorf("failed to start container: %v", err)
  }

  if prev != nil && prev.Image != comp.ContainerConfig.Image {
    if err := self.CleanupImage(prev); err != nil {
      glog.Errorf("failed to cleanup previous image: %v", err)
    }
  }

  self.performPostOp(comp, funcs...)
  return nil
}

func (self *DockerAdapter) performPostOp(comp *manifest.Component,
  funcs... PostSetupFn) {
  glog.Infof("%s", common.CurrentScope())

  for i, fn := range funcs {
    glog.Infof("[%d] execute post setup", i)
    if err := fn(comp); err != nil {
      e := fmt.Sprintf("[%d] execution failed: %v", i, err)
      glog.Error(e)
    }
  }
}

func(self *DockerAdapter) CopyToContainer(cont *types.Container, src_path, dest_path string) error {
  glog.Infof("%s (%s => container:%s)", common.CurrentScope(), src_path, dest_path)

  rd, err := os.OpenFile(src_path, os.O_RDONLY, 0600)
  if err != nil { return err }

  if err := self.cli.CopyToContainer(self.ctx, cont.ID, dest_path, rd, types.CopyToContainerOptions{
    AllowOverwriteDirWithFile: true, CopyUIDGID: false,}); err != nil {
    return err
  }

  return nil
}

func (self *DockerAdapter) CopyFromContainer(cont *types.Container, src, dest string) error {
  glog.Infof("%s (container:%s => %s)", common.CurrentScope(), src, dest)
  body, stat, err := self.cli.CopyFromContainer(self.ctx, cont.ID, src)
  if err != nil {
    return fmt.Errorf("failed to copy updater content: %f", err)
  }
  defer body.Close()
  glog.Info(stat)

  os.RemoveAll(dest)

  wr, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE, 0600)
  if err != nil { return err }
  defer wr.Close()

  if _, err := io.Copy(wr, body); err != nil {
    return err
  }
  return nil
}

func (self *DockerAdapter) getAuthStr(comp *manifest.Component) (string, error) {
  glog.Infof("%s", common.CurrentScope())

  cred, err := manifest.GetCred(comp.Cred)
  if err != nil {
    return "", err
  }

  auth_cfg := types.AuthConfig{
      Username: cred.User,
      Password: cred.Pass,
      ServerAddress: comp.Registry,
  }

  auth_json, err := json.Marshal(auth_cfg)
  if err != nil {
    return "", err
  }

  return base64.URLEncoding.EncodeToString(auth_json), nil
}

func (self *DockerAdapter) FetchImage(comp *manifest.Component) error {
  glog.Infof("%s", common.CurrentScope())

  auth_str, err := self.getAuthStr(comp)
  if err != nil { return err }

  body, err := self.cli.ImagePull(self.ctx, comp.ContainerConfig.Image, types.ImagePullOptions{
    //All: true,
		RegistryAuth: auth_str,
  })

  if err != nil { return err }
  defer body.Close()

  io.Copy(os.Stdout, body)
  return nil
}

func (self *DockerAdapter) DeprecateComponent(comp *manifest.Component) {
  glog.Infof("%s", common.CurrentScope())

  prev, err := self.GetContainersByName(comp.ContainerName)
  if err != nil {
    glog.Infof("faile to list container: %v", err)
  }

  if prev != nil {
    if err := self.CleanupContainer(prev); err != nil {
      glog.Errorf("failed to cleanup previous container: %v", err)
    }
  }
}

func (self *DockerAdapter) CleanupContainer(cont *types.Container) error {
  glog.Infof("%s (%v)", common.CurrentScope(), cont)

  if cont == nil { return nil }

  if err := self.cli.ContainerRemove(self.ctx, cont.ID, types.ContainerRemoveOptions{
      Force: true,}); err != nil {
    return err
  }
  return nil
}

func (self *DockerAdapter) GetContainersByName(name string) (*types.Container, error) {
  glog.Infof("%s", common.CurrentScope())
  name = fmt.Sprintf("/%s", name)

  args := filters.NewArgs()
  args.Add("name", name) // TODO fix filter
  containers, err := self.cli.ContainerList(self.ctx, types.ContainerListOptions{
    All: true,
    Filters: args,
  })
  if err != nil {
    return nil, err
  }

  if len(containers) < 1 {
    return nil, nil
  }

  for _, cont := range containers {
    glog.Infof("%s: %v", name, cont.Names)
    for i := 0; i < len(cont.Names); i++ {
      if cont.Names[i] == name {
        return &cont, nil
      }
    }
  }

  return nil, fmt.Errorf("%s: container not found", name)
}

func (self *DockerAdapter) NeedUpdate(comp *manifest.Component) bool {
  glog.Infof("%s", common.CurrentScope())
  var err error
  if comp.Force { return true }

  if comp.Op == manifest.COMPOP_DEPRECATE {
    self.DeprecateComponent(comp)
    return false
  }

  cont, err := self.GetContainersByName(comp.ContainerName)
  if err != nil {
    glog.Errorf("faile to list container: %s", err)
    return true
  }

  if cont == nil { return true }
  if cont.State != string(containerd.Running) { return true }
  if cont.Image == comp.ContainerConfig.Image { return false }
  return true
}

func (self *DockerAdapter) ListContainers() ([]types.Container, error) {
  containers, err := self.cli.ContainerList(self.ctx, types.ContainerListOptions{All: true,})
  if err != nil {
    return nil, err
  }
  return containers, nil
}

func (self *DockerAdapter) ListImages() ([]types.ImageSummary, error) {
  images, err := self.cli.ImageList(self.ctx, types.ImageListOptions{All: true,})
  if err != nil {
    return nil, err
  }
  return images, nil
}

func (self *DockerAdapter) CleanupImage(cont *types.Container) error {
  glog.Infof("%s (%v)", common.CurrentScope(), cont)

  if cont == nil { return nil }

  rsp, err := self.cli.ImageRemove(self.ctx, cont.Image,
    types.ImageRemoveOptions{Force: true,})
  if err != nil { return err }

  for _, item := range rsp {
    glog.Infof("image delete response: %v", item)
  }
  return nil
}

func (self *DockerAdapter) StartContainer(comp *manifest.Component) error {
  glog.Infof("%s", common.CurrentScope())

  body, err := self.cli.ContainerCreate(self.ctx, &comp.ContainerConfig, &comp.HostConfig, &comp.NetConfig, comp.ContainerName)
  if err != nil { return err }

  if err := self.cli.ContainerStart(self.ctx, body.ID, types.ContainerStartOptions{}); err != nil {
    return err
  }

  return nil
}

func (self *DockerAdapter) BackupContainer(comp *manifest.Component) (*types.Container, error) {
  glog.Infof("%s", common.CurrentScope())

  cont, err := self.GetContainersByName(comp.ContainerName)
  if err != nil {
    return nil, err
  }

  if cont == nil {
    return nil, nil
  }

  backup_name := containerBackupName(cont.Names[0])
  err = self.cli.ContainerRename(self.ctx, cont.ID, backup_name)
  if err != nil {
    glog.Errorf("failed to rename container: ", err)
    return nil, err
  }

  cont, err = self.GetContainersByName(backup_name)
  if err != nil {
    return nil, err
  }

  return cont, nil
}
