package updater

import (
  "fmt"
  "os"
  "encoding/json"
  "sync"
  "time"
  "os/exec"
  "path/filepath"
  "io/ioutil"
  "github.com/golang/glog"
  "github.com/docker/docker/api/types"
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
  UPDATER_POST_OP = "/opt/.updater_post_op"
  UPDATER_SERVICE = os.Getenv("UPDATER_SERVICE")
)

type Daemon struct {
  sched_mutex *sync.Mutex
  setup_mutex *sync.Mutex
  sub *mq.Sub
  sched *sched.Sched
  up *Updater
}

func NewDaemon() *Daemon {
  return &Daemon {
    sched_mutex: &sync.Mutex{},
    setup_mutex: &sync.Mutex{},
    up: NewUpdater(),
  }
}

// Fetch manifest
func (dm *Daemon) fetchMani() (*manifest.UpdateManifest, error) {
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
func (dm *Daemon) RunOnce() {
  dm.sched_mutex.Lock()
  defer dm.sched_mutex.Unlock()

  glog.Infof("%s", common.CurrentScope())

  mani, err := dm.fetchMani()
  if err != nil {
    glog.Error(err)
    return
  }

  dm.setupComponents(mani)
}

func (dm *Daemon) Handle(msg mqtt.Message) {
  glog.Infof("%s", common.CurrentScope())
  data := msg.Payload()

  var mani manifest.UpdateManifest
  if err := json.Unmarshal(data, &mani); err != nil {
    glog.Error("failed to parse json: ", err)
    return
  }
  dm.setupComponents(&mani)
}

func (dm *Daemon) startSub() {
  glog.Infof("%s", common.CurrentScope())
  dm.sub = mq.NewSub(dm)
  dm.sub.SubUpdate()
  go dm.pubStarted()
  dm.sub.StartSub()
}

func (dm *Daemon) startSched() {
  glog.Infof("%s", common.CurrentScope())
  dm.RunOnce()

  dm.sched = sched.NewSched(dm)
  dm.sched.StartSched()
}

func (dm *Daemon) startDual() {
  glog.Infof("%s", common.CurrentScope())
  go dm.startSub()
  dm.startSched()
}

func (dm *Daemon) pubStarted() {
  glog.Infof("%s", common.CurrentScope())

  ev := common.NewEvent()
  ev.Publisher = dm.sub
  ev.Ty = common.EventTypeStarted
  ev.Publish()
}

func (dm *Daemon) Start() {
  glog.Infof("%s", common.CurrentScope())

  switch (os.Getenv("WORK_MODE")) {
  case WORK_MODE_SUB:
    dm.startSub()
  case WORK_MODE_SCHED:
    dm.startSched()
  case WORK_MODE_DUAL:
    dm.startDual()
  default:
    dm.startDual()
  }
}

func (dm *Daemon) onUpdaterPostOp() bool {
  glog.Infof("%s", common.CurrentScope())
  fd, err := os.OpenFile(UPDATER_POST_OP, os.O_RDONLY, 0600)
  if err == nil {
    defer fd.Close()
    glog.Infof("updater post op")
    return true
  }
  glog.Error(err)
  return false
}

func (dm *Daemon) pub_error(e string) {
  ev := common.NewErrEvent(e)
  ev.Publisher = dm.sub
  ev.Publish()
}

func (dm *Daemon) setUpdaterPostOp() error {
  glog.Infof("%s", common.CurrentScope())
  now, err := time.Now().MarshalText()
  if err != nil {
    return err
  }
  return ioutil.WriteFile(UPDATER_POST_OP, now, 0600)
}

func (dm *Daemon) setupComponents(mani *manifest.UpdateManifest) {
  glog.Infof("%s", common.CurrentScope())
  if err := detectEnv(); err != nil {
    glog.Error(err)
    return
  }

  dm.setup_mutex.Lock()

  for i, comp := range mani.Components {
    glog.Infof("[%d] setup %v", i, comp)
    emsg := ""

    switch comp.Name {
    case manifest.COMP_UPDATER:
      if dm.onUpdaterPostOp() {
        os.RemoveAll(UPDATER_POST_OP)
        if err := dm.up.SetupContainer(&comp, true,
            dm.PostSetupDB); err != nil {
          emsg = fmt.Sprintf("failed to setup container: %v", err)
          glog.Error(emsg)
        }
      } else if !dm.up.NeedUpdate(&comp) {
        continue
      } else {
        if err := dm.setUpdaterPostOp(); err != nil {
          emsg := fmt.Sprintf("failed to set updater post op: %v", err)
          glog.Error(emsg)
        }

        if err := dm.up.SetupContainer(&comp, false,
            dm.PostSetupUpdater,
            // updater exits after deploy
            dm.PostSetupUpdaterDeploy); err != nil {
          emsg := fmt.Sprintf("failed to setup container: %v", err)
          glog.Error(emsg)
        }
      }
    default:
      if err := dm.up.SetupContainer(&comp, false); err != nil {
        emsg = fmt.Sprintf("failed to setup container: %v", err)
        glog.Error(emsg)
      }
    }

    if len(emsg) != 0 { dm.pub_error(emsg) }
  }

  if err := dm.heartbeatUpdate(); err != nil {
    glog.Errorf("heartbeat failed: %v", err)
  }
  dm.setup_mutex.Unlock()
}

// Post Operation callback
func (dm *Daemon) PostSetupUpdater(comp *manifest.Component) error {
  glog.Infof("%s", common.CurrentScope())

  cont, err := dm.up.getContainersByName(comp.ContainerName)
  if err != nil { return err }

  if err := dm.extractUpdaterContent(cont); err != nil {
    return err
  }

  if err := dm.deployUpdater(); err != nil {
    return err
  }

  return nil
}


func (dm *Daemon) deployUpdater() error {
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

// Post Operation callback
func (dm *Daemon) PostSetupUpdaterDeploy(comp *manifest.Component) error {
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
  dm.setup_mutex.Unlock()
  os.Exit(1)

  return nil
}

func (dm *Daemon) PostSetupDB(comp *manifest.Component) error {
  glog.Infof("%s", common.CurrentScope())

  cont, err := dm.up.getContainersByName(comp.ContainerName)
  if err != nil { return err }

  if err := dm.updateDatabase(cont); err != nil {
    return err
  }
  return nil
}

func (dm *Daemon) extractUpdaterContent(cont *types.Container) (error) {
  glog.Infof("%s", common.CurrentScope())

  src_path := UPDATER_IN_CONTAINER
  dest_path := filepath.Join("/tmp", filepath.Base(src_path))

  if err := dm.up.copyFromContainer(cont, src_path, dest_path); err != nil {
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

func (dm *Daemon) updateDatabase(cont *types.Container) (error) {
  glog.Infof("%s", common.CurrentScope())

  if err := dm.execDatabaseUpdate(cont, UPDATE_SQL_PATH); err != nil {
    return err
  }
  return nil
}

func (daemon *Daemon) execDatabaseUpdate(cont *types.Container, sql_path string) error {
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

func (dm *Daemon) heartbeatUpdate() error {
  glog.Infof("%s", common.CurrentScope())
  var err error

  hb := common.NewUpdaterHeartbeat()
  hb.Publisher = dm.sub

  if hb.Containers, err = dm.up.ListContainers(); err != nil {
    hb.Error = fmt.Sprintf("failed to list containers", err)
    return hb.Publish()
  }

  if hb.Images, err = dm.up.ListImages(); err != nil {
    hb.Error = fmt.Sprintf("failed to list images", err)
    return hb.Publish()
  }

  return hb.Publish()
}
