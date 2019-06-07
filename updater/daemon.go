package updater

import (
  "fmt"
  "os"
  "encoding/json"
  "sync"
  "github.com/golang/glog"
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

type Daemon struct {
  sched_mutex *sync.Mutex
  sub *mq.Sub
  sched *sched.Sched
  up IUpdater
}

func NewDaemon() *Daemon {
  ret := &Daemon {
    sched_mutex: &sync.Mutex{},
  }
  ret.sub = mq.NewSub(ret)
  ret.up = NewDockerUpdater(ret.sub)
  return ret
}

// Fetch manifest
func (self *Daemon) fetchMani() (*manifest.UpdateManifest, error) {
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
func (self *Daemon) RunOnce() {
  self.sched_mutex.Lock()
  defer self.sched_mutex.Unlock()

  glog.Infof("%s", common.CurrentScope())

  mani, err := self.fetchMani()
  if err != nil {
    glog.Error(err)
    return
  }

  self.up.SetupComponents(mani)
}

// MQ message handler
func (self *Daemon) Handle(msg mqtt.Message) {
  glog.Infof("%s", common.CurrentScope())
  data := msg.Payload()

  var mani manifest.UpdateManifest
  if err := json.Unmarshal(data, &mani); err != nil {
    glog.Error("failed to parse json: ", err)
    return
  }

  self.up.SetupComponents(&mani)
}

func (self *Daemon) startSub() {
  glog.Infof("%s", common.CurrentScope())
  self.sub.SubUpdate()
  go self.pubStarted()
  self.sub.StartSub()
}

func (self *Daemon) startSched() {
  glog.Infof("%s", common.CurrentScope())
  self.RunOnce()

  self.sched = sched.NewSched(self)
  self.sched.StartSched()
}

func (self *Daemon) startDual() {
  glog.Infof("%s", common.CurrentScope())
  go self.startSub()
  self.startSched()
}

func (self *Daemon) pubStarted() {
  glog.Infof("%s", common.CurrentScope())

  ev := common.NewEvent()
  ev.Publisher = self.sub
  ev.Ty = common.EventTypeStarted
  ev.Publish()
}

func (self *Daemon) Start() {
  glog.Infof("%s", common.CurrentScope())

  switch (os.Getenv("WORK_MODE")) {
  case WORK_MODE_SUB:
    self.startSub()
  case WORK_MODE_SCHED:
    self.startSched()
  case WORK_MODE_DUAL:
    self.startDual()
  default:
    self.startDual()
  }
}
/** Database post op
var (
  // on host
  //UPDATE_SQL_PATH = os.Getenv("UPDATE_SQL_PATH")
)

func (self *Daemon) PostSetupDB(comp *manifest.Component) error {
  glog.Infof("%s", common.CurrentScope())

  cont, err := self.up.GetContainersByName(comp.ContainerName)
  if err != nil { return err }

  if err := self.updateDatabase(cont); err != nil {
    return err
  }
  return nil
}

func (self *Daemon) updateDatabase(cont *types.Container) (error) {
  glog.Infof("%s", common.CurrentScope())

  if err := self.execDatabaseUpdate(cont, UPDATE_SQL_PATH); err != nil {
    return err
  }
  return nil
}

func (self *Daemon) execDatabaseUpdate(cont *types.Container, sql_path string) error {
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
*/
