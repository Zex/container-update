package gen

import (
  "flag"
  "os"
  "fmt"
  "strconv"
  "github.com/docker/docker/api/types/container"
  //"github.com/docker/go-connections/nat"
  "github.com/zex/container-update/common"
  "github.com/zex/container-update/manifest"
)

type Gen struct {
  Reg *common.Registry
}

var (
  RegAuth = os.Getenv("DREG_AUTH")
  RegHost = flag.String("r", "10.10.0.33:17889", "Docker registry")
  AuthSvr = flag.String("a", "", "Docker registry auth server")
  ImgName = flag.String("n", "", "Image name")
)

// names of known components
const (
  COMP_APP = "application"
  COMP_DB = "db"
)

func NewGen() *Gen {
  return &Gen{
    Reg: common.NewRegistry(RegAuth, *RegHost, *AuthSvr),
    }
}

func (g *Gen) NewAppComp(image_name, version string) (*manifest.Component) {
  tag := "1.0"//g.LatestTag(image_name)
  cred, _ := manifest.GenCred(g.Reg.GetUserPasswd())

  force := false
  if os.Getenv("APP_FORCE") != "" {
    f, err := strconv.ParseBool(os.Getenv("APP_FORCE"))
    if err == nil { force = f
    } else { fmt.Println(err) }
  }

  comp := manifest.Component{
    Version: version,
    Name: COMP_APP,
    Registry: g.Reg.Host,
    ImageName: image_name,
    ImageTag: tag,
    Cred: cred,
    Force: force,
    Op: manifest.COMPOP_UPDATE,
    ContainerName: "application",
    ContainerConfig: container.Config {
      Image: fmt.Sprintf("%s/%s:%s", g.Reg.Host, image_name, tag),
      Tty: false,
      AttachStdin: false,
      AttachStdout: false,
      AttachStderr: false,
      Env: []string{
        fmt.Sprintf("MODELS_VERSION=%s", os.Getenv("MODELS_VERSION")),
      },
    },
    HostConfig: container.HostConfig {
      RestartPolicy: container.RestartPolicy{Name: "always",},
      NetworkMode: "bridge",
      Links: []string{
        "mysql:/application/mysql",
      },
      Privileged: true,
      Binds: []string{
        "/application-data:/data",
        "/opt/application/config/application.yaml:/opt/application/config/application.yaml",
      },
    },
  }

  // /dev/nvidia0  /dev/nvidiactl  /dev/nvidia-uvm
  comp.HostConfig.Resources.Devices = []container.DeviceMapping{ // TODO
    {PathOnHost: "/dev/nvidia0", PathInContainer: "/dev/nvidia0", CgroupPermissions: "rwm"},
    {PathOnHost: "/dev/nvidiactl", PathInContainer: "/dev/nvidiactl", CgroupPermissions: "rwm"},
    {PathOnHost: "/dev/nvidia-uvm", PathInContainer: "/dev/nvidia-uvm", CgroupPermissions: "rwm"},
  }

  return &comp
}

func (g *Gen) NewDbComp(image_name, version string) (*manifest.Component) {
  tag := "1.0"//g.LatestTag(image_name)
  cred, _ := manifest.GenCred(g.Reg.GetUserPasswd())

  comp := manifest.Component{
    Version: version,
    Name: COMP_DB,
    Registry: g.Reg.Host,
    ImageName: image_name,
    ImageTag: tag,
    Cred: cred,
    Op: manifest.COMPOP_UPDATE,
    ContainerName: "mysql",
    ContainerConfig: container.Config {
      Image: fmt.Sprintf("%s/%s:%s", g.Reg.Host, image_name, tag),
      Tty: false,
      AttachStdin: false,
      AttachStdout: false,
      AttachStderr: false,
      /**
      ExposedPorts: nat.PortSet{
        nat.Port(fmt.Sprintf("%d/tcp", 3306)):{},
      },
      */
    },
    HostConfig: container.HostConfig {
      RestartPolicy: container.RestartPolicy{Name: "always",},
      /**
      PortBindings: nat.PortMap{
        nat.Port(fmt.Sprintf("%d/tcp", 3306)): []nat.PortBinding{
          {HostIP: "0.0.0.0", HostPort: "13306",},},
      },
      */
      Binds: []string{"mysql-data:/var/lib/mysql:rw"},
    },
  }

  return &comp
}

func (g *Gen) NewUpdaterComp(image_name, version string) (*manifest.Component) {
  tag := "1.0"//g.LatestTag(image_name)
  cred, _ := manifest.GenCred(g.Reg.GetUserPasswd())

  comp := manifest.Component{
    Version: version,
    Name: manifest.COMP_UPDATER,
    Registry: g.Reg.Host,
    ImageName: image_name,
    ImageTag: tag,
    Cred: cred,
    Op: manifest.COMPOP_UPDATE,
    ContainerName: "updater",
    ContainerConfig: container.Config {
      Image: fmt.Sprintf("%s/%s:%s", g.Reg.Host, image_name, tag),
      Tty: true,
      AttachStdin: false,
      AttachStdout: false,
      AttachStderr: false,
      Entrypoint: []string{"/bin/bash"},
    },
  }

  return &comp
}

func (g *Gen) LatestTag(image_name string) string {
  tag, err := g.Reg.GetLatestTag(image_name)
  if err != nil { panic(err) }
  return tag
}
