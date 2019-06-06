package updater

import (
  "fmt"
  "os"
  "io"
  "path/filepath"
  "context"
  "encoding/json"
  "encoding/base64"
  "sync"
  "io/ioutil"
  "time"
  "os/exec"
  "github.com/golang/glog"
  "github.com/docker/docker/api/types"
  "github.com/docker/docker/api/types/filters"
  //"github.com/docker/docker/libcontainerd"
	"github.com/containerd/containerd"
	"github.com/docker/docker/pkg/parsers/operatingsystem"
  docker "github.com/docker/docker/client"
  "github.com/zex/container-update/manifest"
  "github.com/zex/container-update/common"
  mq "github.com/zex/container-update/mqtt"
  "github.com/eclipse/paho.mqtt.golang"
  sched "github.com/zex/container-update/sched"
)

const (
  WORK_MODE_SUB = "sub"
  WORK_MODE_SCHED = "sched"
  WORK_MODE_DUAL = "dual"
)

var (
  UPDATER_IN_CONTAINER = os.Getenv("UPDATER_IN_CONTAINER")
  // on host
  UPDATE_SQL_PATH = os.Getenv("UPDATE_SQL_PATH")
  UPDATER_ROOT = os.Getenv("UPDATER_ROOT")
  UPDATER_SERVICE = os.Getenv("UPDATER_SERVICE")
  UPDATER_POST_OP = "/opt/.updater_post_op"
)

type Updater struct {
  ctx context.Context
  sched_mutex *sync.Mutex
  setup_mutex *sync.Mutex
  sub *mq.Sub
  sched *sched.Sched
}

type PostSetupFn func(comp *manifest.Component, cli *docker.Client) error

func NewUpdater() *Updater {
  return &Updater{
    ctx: context.Background(),
    sched_mutex: &sync.Mutex{},
    setup_mutex: &sync.Mutex{},
  }
}

// Fetch manifest
func (up *Updater) fetchMani() (*manifest.UpdateManifest, error) {
  glog.Infof("%s", common.CurrentScope())

  asset_mani := os.Getenv("ASSET_MANIFEST")
  if asset_mani == "" {
    return nil, fmt.Errorf("asset manifest not given")
  }

  asset, err := manifest.DecodeAsset(asset_mani)
  if err != nil {
    return nil, err
  }

  return manifest.FetchMani(asset.Url)
}

// interface TimeoutHandler callback
func (up *Updater) RunOnce() {
  up.sched_mutex.Lock()
  defer up.sched_mutex.Unlock()

  glog.Infof("%s", common.CurrentScope())

  mani, err := up.fetchMani()
  if err != nil {
    glog.Error(err)
    return
  }

  up.setupComponents(mani)
}

func (up *Updater) Handle(msg mqtt.Message) {
  glog.Infof("%s", common.CurrentScope())
  data := msg.Payload()

  var mani manifest.UpdateManifest
  if err := json.Unmarshal(data, &mani); err != nil {
    glog.Error("failed to parse json: ", err)
    return
  }
  up.setupComponents(&mani)
}

func (up *Updater) startSub() {
  glog.Infof("%s", common.CurrentScope())
  up.sub = mq.NewSub(up)
  up.sub.SubUpdate()
  go up.pubStarted()
  up.sub.StartSub()
}

func (up *Updater) startSched() {
  glog.Infof("%s", common.CurrentScope())
  up.RunOnce()

  up.sched = sched.NewSched(up)
  up.sched.StartSched()
}

func (up *Updater) startDual() {
  glog.Infof("%s", common.CurrentScope())
  go up.startSub()
  up.startSched()
}

func (up *Updater) pubStarted() {
  glog.Infof("%s", common.CurrentScope())

  ev := common.NewEvent()
  ev.Publisher = up.sub
  ev.Ty = common.EventTypeStarted
  ev.Publish()
}

func (up *Updater) Start() {
  glog.Infof("%s", common.CurrentScope())

  switch (os.Getenv("WORK_MODE")) {
  case WORK_MODE_SUB:
    up.startSub()
  case WORK_MODE_SCHED:
    up.startSched()
  case WORK_MODE_DUAL:
    up.startDual()
  default:
    up.startDual()
  }
}

func (up *Updater) onUpdaterPostOp() bool {
  glog.Infof("%s", common.CurrentScope())
  fd, err := os.OpenFile(UPDATER_POST_OP, os.O_RDONLY, 0600)
  if err == nil {
    defer fd.Close()
    glog.Infof("updater post op")
    return true
  }
  glog.Info(err)
  return false
}

func (up *Updater) setUpdaterPostOp() error {
  glog.Infof("%s", common.CurrentScope())
  now, err := time.Now().MarshalText()
  if err != nil { return err }
  return ioutil.WriteFile(UPDATER_POST_OP, now, 0600)
}

func detectEnv() error {
  if in_cont, err := operatingsystem.IsContainerized(); err != nil {
    glog.Infof("cannot detect whether we are containerized")
  } else if in_cont {
    return fmt.Errorf("updater should not be running in container")
  }
  return nil
}

func (up *Updater) pub_error(e string) {
  ev := common.NewErrEvent(e)
  ev.Publisher = up.sub
  ev.Publish()
}

func (up *Updater) setupComponents(mani *manifest.UpdateManifest) {
  glog.Infof("%s", common.CurrentScope())
  if err := detectEnv(); err != nil {
    glog.Error(err)
    return
  }

  up.setup_mutex.Lock()

  for i, comp := range mani.Components {
    glog.Infof("[%d] setup %v", i, comp)
    emsg := ""

    switch comp.Name {
    case manifest.COMP_UPDATER:
      if up.onUpdaterPostOp() {
        os.RemoveAll(UPDATER_POST_OP)
        if err := up.SetupContainer(&comp, true,
            up.PostSetupDB); err != nil {
          emsg = fmt.Sprintf("failed to setup container: %v", err)
          glog.Error(emsg)
        }
      } else if !up.needUpdate(&comp, nil) {
        continue
      } else {
        if err := up.setUpdaterPostOp(); err != nil {
          emsg := fmt.Sprintf("failed to set updater post op: %v", err)
          glog.Error(emsg)
        }

        if err := up.SetupContainer(&comp, false,
            up.PostSetupUpdater,
            // updater exits after deploy
            up.PostSetupUpdaterDeploy); err != nil {
          emsg := fmt.Sprintf("failed to setup container: %v", err)
          glog.Error(emsg)
        }
      }
    default:
      if err := up.SetupContainer(&comp, false); err != nil {
        emsg = fmt.Sprintf("failed to setup container: %v", err)
        glog.Error(emsg)
      }
    }

    if len(emsg) != 0 { up.pub_error(emsg) }
  }

  if err := up.heartbeatUpdated(); err != nil {
    glog.Errorf("heartbeat failed: %v", err)
  }
  up.setup_mutex.Unlock()
}

func (up *Updater) PostSetupUpdater(comp *manifest.Component, cli *docker.Client) error {
  glog.Infof("%s", common.CurrentScope())

  cont, err := up.getContainersByName(cli, comp.ContainerName)
  if err != nil { return err }

  if err := up.extractUpdaterContent(cli, cont); err != nil {
    return err
  }

  if err := up.deployUpdater(); err != nil {
    return err
  }

  return nil
}


func (up *Updater) deployUpdater() error {
  if err := os.Chmod(filepath.Join(UPDATER_ROOT, "scripts/updater.sh"), 0755); err != nil {
    return err
  }

  if err := common.Copy(filepath.Join(UPDATER_ROOT, fmt.Sprintf("config/%s.service", UPDATER_SERVICE)),
    fmt.Sprintf("/etc/systemd/system/%s.service", UPDATER_SERVICE)); err != nil{
    return err
  }

  updater_path := filepath.Join(UPDATER_ROOT, "build/updater/updater")
  if err := os.Chmod(updater_path, 0755); err != nil {
    return err
  }
  return nil
}

func (up *Updater) PostSetupUpdaterDeploy(comp *manifest.Component, cli *docker.Client) error {
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
  up.setup_mutex.Unlock()
  os.Exit(1)

  return nil
}

func (up *Updater) PostSetupDB(comp *manifest.Component, cli *docker.Client) error {
  glog.Infof("%s", common.CurrentScope())

  cont, err := up.getContainersByName(cli, comp.ContainerName)
  if err != nil { return err }

  if err := up.updateDatabase(cli, cont); err != nil {
    return err
  }
  return nil
}

func (up *Updater) extractUpdaterContent(cli *docker.Client, cont *types.Container) (error) {
  glog.Infof("%s", common.CurrentScope())

  src_path := UPDATER_IN_CONTAINER
  dest_path := filepath.Join("/tmp", filepath.Base(src_path))

  if err := up.copyFromContainer(cli, cont, src_path, dest_path); err != nil {
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

func (up *Updater) updateDatabase(cli *docker.Client, cont *types.Container) (error) {
  glog.Infof("%s", common.CurrentScope())

  if err := up.execDatabaseUpdate(cli, cont, UPDATE_SQL_PATH); err != nil {
    return err
  }
  return nil
}

func(up *Updater) copyToContainer(cli *docker.Client, cont *types.Container, src_path, dest_path string) error {
  glog.Infof("%s (%s => container:%s)", common.CurrentScope(), src_path, dest_path)

  rd, err := os.OpenFile(src_path, os.O_RDONLY, 0600)
  if err != nil { return err }

  if err := cli.CopyToContainer(up.ctx, cont.ID, dest_path, rd, types.CopyToContainerOptions{
    AllowOverwriteDirWithFile: true, CopyUIDGID: false,}); err != nil {
    return err
  }

  return nil
}

func (up *Updater) execDatabaseUpdate(cli *docker.Client, cont *types.Container, sql_path string) error {
  glog.Infof("%s (%s)", common.CurrentScope(), sql_path)

  var (
    db_user = os.Getenv("DB_LOGIN")
    db_key = os.Getenv("DB_KEY")
    db_port = os.Getenv("DB_PORT")
    db_host = os.Getenv("DB_HOST")
  )

  pass, err := ioutil.ReadFile(db_key)
  if err != nil { return err }

  return common.NativeMysql(db_user, db_host, db_port, string(pass), sql_path)
}

func (up *Updater) copyFromContainer(cli *docker.Client, cont *types.Container,
        src, dest string) error {
  glog.Infof("%s (container:%s => %s)", common.CurrentScope(), src, dest)
  body, stat, err := cli.CopyFromContainer(up.ctx, cont.ID, src)
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

func (up *Updater) performPostOp(cli *docker.Client, comp *manifest.Component,
  funcs... PostSetupFn) {
  glog.Infof("%s", common.CurrentScope())

  for i, fn := range funcs {
    glog.Infof("[%d] execute post setup", i)
    if err := fn(comp, cli); err != nil {
      e := fmt.Sprintf("[%d] execution failed: %v", i, err)
      glog.Error(e)
      up.pub_error(e)
    }
  }
}

func (up *Updater) SetupContainer(comp *manifest.Component, post_only bool, funcs... PostSetupFn) (error) {
  glog.Infof("%s (post_only=%d)", common.CurrentScope(), post_only)

  cli, err := docker.NewEnvClient()
  if err != nil { return err }

  if !up.needUpdate(comp, cli) {
    if post_only { up.performPostOp(cli, comp, funcs...) }
    glog.Infof("%s: update not needed", comp.Name)
    return nil
  }

  if err := up.fetchImage(cli, comp); err != nil {
    return fmt.Errorf("failed to pull image: %v", err)
  }

  prev, err := up.getContainersByName(cli, comp.ContainerName)
  if err != nil {
    glog.Infof("faile to list container: %v", err)
  }

  if prev != nil {
    if err := up.cleanupContainer(cli, prev); err != nil {
      glog.Errorf("failed to cleanup previous container: %v", err)
    }
  }

  /**
  prev, err := up.backupContainer(cli, comp)
  if err != nil {
    return err
  }
  */

  if err := up.startContainer(cli, comp); err != nil {
    return fmt.Errorf("failed to start container: %v", err)
  }

  if prev != nil && prev.Image != comp.ContainerConfig.Image {
    if err := up.cleanupImage(cli, prev); err != nil {
      glog.Errorf("failed to cleanup previous image: %v", err)
    }
  }

  up.performPostOp(cli, comp, funcs...)
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

func (up *Updater) fetchImage(cli *docker.Client, comp *manifest.Component) (error) {
  glog.Infof("%s", common.CurrentScope())

  auth_str, err := up.getAuthStr(comp)
  if err != nil { return err }

  body, err := cli.ImagePull(up.ctx, comp.ContainerConfig.Image, types.ImagePullOptions{
    //All: true,
		RegistryAuth: auth_str,
  })

  if err != nil { return err }
  defer body.Close()

  io.Copy(os.Stdout, body)
  return nil
}

func containerBackupName(name string) string {
  return fmt.Sprintf("%s-prev", name)
}

func (up *Updater) deprecateComponent(comp *manifest.Component, cli *docker.Client) {
  glog.Infof("%s", common.CurrentScope())

  prev, err := up.getContainersByName(cli, comp.ContainerName)
  if err != nil {
    glog.Infof("faile to list container: %v", err)
  }

  if prev != nil {
    if err := up.cleanupContainer(cli, prev); err != nil {
      glog.Errorf("failed to cleanup previous container: %v", err)
    }
  }
}

func (up *Updater) needUpdate(comp *manifest.Component, cli *docker.Client) bool {
  glog.Infof("%s", common.CurrentScope())
  var err error
  if comp.Force { return true }

  if comp.Op == manifest.COMPOP_DEPRECATE {
    up.deprecateComponent(comp, cli)
    return false
  }

  if cli == nil {
    cli, err = docker.NewEnvClient()
    if err != nil { return true }
  }

  cont, err := up.getContainersByName(cli, comp.ContainerName)
  if err != nil {
    glog.Errorf("faile to list container: %s", err)
    return true
  }

  if cont == nil { return true }
  if cont.State != string(containerd.Running) { return true }
  if cont.Image == comp.ContainerConfig.Image { return false }
  return true
}

func (up *Updater) backupContainer(cli *docker.Client, comp *manifest.Component) (*types.Container, error) {
  glog.Infof("%s", common.CurrentScope())

  cont, err := up.getContainersByName(cli, comp.ContainerName)
  if err != nil {
    return nil, err
  }

  if cont == nil {
    return nil, nil
  }

  backup_name := containerBackupName(cont.Names[0])
  err = cli.ContainerRename(up.ctx, cont.ID, backup_name)
  if err != nil {
    glog.Errorf("failed to rename container: ", err)
    return nil, err
  }

  cont, err = up.getContainersByName(cli, backup_name)
  if err != nil {
    return nil, err
  }

  return cont, nil
}

func (up *Updater) heartbeatUpdated() error {
  glog.Infof("%s", common.CurrentScope())

  hb := common.NewUpdaterHeartbeat()
  hb.Publisher = up.sub

  cli, err := docker.NewEnvClient()
  if err != nil {
    hb.Error = fmt.Sprintf("failed to create docker client", err)
    return hb.Publish()
  }

  containers, err := cli.ContainerList(up.ctx, types.ContainerListOptions{All: true,})
  if err != nil {
    hb.Error = fmt.Sprintf("failed to list containers", err)
    return hb.Publish()
  }
  hb.Containers = containers

  images, err := cli.ImageList(up.ctx, types.ImageListOptions{All: true,})
  if err != nil {
    hb.Error = fmt.Sprintf("failed to list images", err)
    return hb.Publish()
  }
  hb.Images = images

  return hb.Publish()
}

func (up *Updater) getContainersByName(cli *docker.Client, name string) (*types.Container, error) {
  glog.Infof("%s", common.CurrentScope())
  name = fmt.Sprintf("/%s", name)

  args := filters.NewArgs()
  args.Add("name", name) // TODO fix filter
  containers, err := cli.ContainerList(up.ctx, types.ContainerListOptions{
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

func (up *Updater) cleanupContainer(cli *docker.Client, cont *types.Container) error {
  glog.Infof("%s (%v)", common.CurrentScope(), cont)

  if cont == nil { return nil }

  if err := cli.ContainerRemove(up.ctx, cont.ID, types.ContainerRemoveOptions{
      Force: true,}); err != nil {
    return err
  }
  return nil
}

func (up *Updater) cleanupImage(cli *docker.Client, cont *types.Container) error {
  glog.Infof("%s (%v)", common.CurrentScope(), cont)

  if cont == nil { return nil }

  rsp, err := cli.ImageRemove(up.ctx, cont.Image,
    types.ImageRemoveOptions{Force: true,})
  if err != nil { return err }

  for _, item := range rsp {
    glog.Infof("image delete response: %v", item)
  }
  return nil
}

func (up *Updater) startContainer(cli *docker.Client, comp *manifest.Component) (error) {
  glog.Infof("%s", common.CurrentScope())

  body, err := cli.ContainerCreate(up.ctx, &comp.ContainerConfig, &comp.HostConfig, &comp.NetConfig, comp.ContainerName)
  if err != nil { return err }

  if err := cli.ContainerStart(up.ctx, body.ID, types.ContainerStartOptions{}); err != nil {
    return err
  }

  return nil
}
