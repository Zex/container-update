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

var (
  UPDATER_IN_CONTAINER = os.Getenv("UPDATER_IN_CONTAINER")
  // on host
  UPDATE_SQL_PATH = os.Getenv("UPDATE_SQL_PATH")
  UPDATER_ROOT = os.Getenv("UPDATER_ROOT")
)

type PostSetupFn func(comp *manifest.Component) error

func detectEnv() error {
  if in_cont, err := operatingsystem.IsContainerized(); err != nil {
    glog.Infof("cannot detect whether we are containerized")
  } else if in_cont {
    return fmt.Errorf("updater should not be running in container")
  }
  return nil
}

type Updater struct {
  ctx context.Context
  cli *docker.Client
}

func NewUpdater() *Updater {
  var err error
  ret := &Updater{
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

func (up *Updater) GetDClient() error {
  if up.cli != nil {
    return nil
  }

  var err error
  up.cli, err = docker.NewEnvClient()
  if err != nil {
    return err
  }
  return nil
}

func (up *Updater) SetupContainer(comp *manifest.Component, post_only bool, funcs... PostSetupFn) (error) {
  glog.Infof("%s (post_only=%d)", common.CurrentScope(), post_only)

  if err := up.GetDClient(); err != nil {
    return err
  }

  if !up.NeedUpdate(comp) {
    if post_only { up.performPostOp(comp, funcs...) }
    glog.Infof("%s: update not needed", comp.Name)
    return nil
  }

  if err := up.fetchImage(comp); err != nil {
    return fmt.Errorf("failed to pull image: %v", err)
  }

  prev, err := up.getContainersByName(comp.ContainerName)
  if err != nil {
    glog.Infof("faile to list container: %v", err)
  }

  if prev != nil {
    if err := up.cleanupContainer(prev); err != nil {
      glog.Errorf("failed to cleanup previous container: %v", err)
    }
  }

  /**
  prev, err := up.backupContainer(comp)
  if err != nil {
    return err
  }
  */

  if err := up.startContainer(comp); err != nil {
    return fmt.Errorf("failed to start container: %v", err)
  }

  if prev != nil && prev.Image != comp.ContainerConfig.Image {
    if err := up.cleanupImage(prev); err != nil {
      glog.Errorf("failed to cleanup previous image: %v", err)
    }
  }

  up.performPostOp(comp, funcs...)
  return nil
}

func (up *Updater) performPostOp(comp *manifest.Component,
  funcs... PostSetupFn) {
  glog.Infof("%s", common.CurrentScope())

  for i, fn := range funcs {
    glog.Infof("[%d] execute post setup", i)
    if err := fn(comp); err != nil {
      e := fmt.Sprintf("[%d] execution failed: %v", i, err)
      glog.Error(e)
      //up.pub_error(e)
    }
  }
}

func(up *Updater) copyToContainer(cont *types.Container, src_path, dest_path string) error {
  glog.Infof("%s (%s => container:%s)", common.CurrentScope(), src_path, dest_path)

  rd, err := os.OpenFile(src_path, os.O_RDONLY, 0600)
  if err != nil { return err }

  if err := up.cli.CopyToContainer(up.ctx, cont.ID, dest_path, rd, types.CopyToContainerOptions{
    AllowOverwriteDirWithFile: true, CopyUIDGID: false,}); err != nil {
    return err
  }

  return nil
}

func (up *Updater) copyFromContainer(cont *types.Container, src, dest string) error {
  glog.Infof("%s (container:%s => %s)", common.CurrentScope(), src, dest)
  body, stat, err := up.cli.CopyFromContainer(up.ctx, cont.ID, src)
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

func (up *Updater) getAuthStr(comp *manifest.Component) (string, error) {
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

func (up *Updater) fetchImage(comp *manifest.Component) (error) {
  glog.Infof("%s", common.CurrentScope())

  auth_str, err := up.getAuthStr(comp)
  if err != nil { return err }

  body, err := up.cli.ImagePull(up.ctx, comp.ContainerConfig.Image, types.ImagePullOptions{
    //All: true,
		RegistryAuth: auth_str,
  })

  if err != nil { return err }
  defer body.Close()

  io.Copy(os.Stdout, body)
  return nil
}

func (up *Updater) deprecateComponent(comp *manifest.Component) {
  glog.Infof("%s", common.CurrentScope())

  prev, err := up.getContainersByName(comp.ContainerName)
  if err != nil {
    glog.Infof("faile to list container: %v", err)
  }

  if prev != nil {
    if err := up.cleanupContainer(prev); err != nil {
      glog.Errorf("failed to cleanup previous container: %v", err)
    }
  }
}

func (up *Updater) cleanupContainer(cont *types.Container) error {
  glog.Infof("%s (%v)", common.CurrentScope(), cont)

  if cont == nil { return nil }

  if err := up.cli.ContainerRemove(up.ctx, cont.ID, types.ContainerRemoveOptions{
      Force: true,}); err != nil {
    return err
  }
  return nil
}

func (up *Updater) getContainersByName(name string) (*types.Container, error) {
  glog.Infof("%s", common.CurrentScope())
  name = fmt.Sprintf("/%s", name)

  args := filters.NewArgs()
  args.Add("name", name) // TODO fix filter
  containers, err := up.cli.ContainerList(up.ctx, types.ContainerListOptions{
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

func (up *Updater) NeedUpdate(comp *manifest.Component) bool {
  glog.Infof("%s", common.CurrentScope())
  var err error
  if comp.Force { return true }

  if comp.Op == manifest.COMPOP_DEPRECATE {
    up.deprecateComponent(comp)
    return false
  }

  cont, err := up.getContainersByName(comp.ContainerName)
  if err != nil {
    glog.Errorf("faile to list container: %s", err)
    return true
  }

  if cont == nil { return true }
  if cont.State != string(containerd.Running) { return true }
  if cont.Image == comp.ContainerConfig.Image { return false }
  return true
}

func (up *Updater) ListContainers() ([]types.Container, error) {
  containers, err := up.cli.ContainerList(up.ctx, types.ContainerListOptions{All: true,})
  if err != nil {
    return nil, err
  }
  return containers, nil
}

func (up *Updater) ListImages() ([]types.ImageSummary, error) {
  images, err := up.cli.ImageList(up.ctx, types.ImageListOptions{All: true,})
  if err != nil {
    return nil, err
  }
  return images, nil
}

func (up *Updater) cleanupImage(cont *types.Container) error {
  glog.Infof("%s (%v)", common.CurrentScope(), cont)

  if cont == nil { return nil }

  rsp, err := up.cli.ImageRemove(up.ctx, cont.Image,
    types.ImageRemoveOptions{Force: true,})
  if err != nil { return err }

  for _, item := range rsp {
    glog.Infof("image delete response: %v", item)
  }
  return nil
}

func (up *Updater) startContainer(comp *manifest.Component) (error) {
  glog.Infof("%s", common.CurrentScope())

  body, err := up.cli.ContainerCreate(up.ctx, &comp.ContainerConfig, &comp.HostConfig, &comp.NetConfig, comp.ContainerName)
  if err != nil { return err }

  if err := up.cli.ContainerStart(up.ctx, body.ID, types.ContainerStartOptions{}); err != nil {
    return err
  }

  return nil
}

func (up *Updater) backupContainer(comp *manifest.Component) (*types.Container, error) {
  glog.Infof("%s", common.CurrentScope())

  cont, err := up.getContainersByName(comp.ContainerName)
  if err != nil {
    return nil, err
  }

  if cont == nil {
    return nil, nil
  }

  backup_name := containerBackupName(cont.Names[0])
  err = up.cli.ContainerRename(up.ctx, cont.ID, backup_name)
  if err != nil {
    glog.Errorf("failed to rename container: ", err)
    return nil, err
  }

  cont, err = up.getContainersByName(backup_name)
  if err != nil {
    return nil, err
  }

  return cont, nil
}
