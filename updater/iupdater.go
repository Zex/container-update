package updater

import (
  "github.com/zex/container-update/manifest"
  "github.com/docker/docker/api/types"
)

type PostSetupFn func(comp *manifest.Component) error

type IDocker interface {
  SetupContainer(comp *manifest.Component, post_only bool, funcs... PostSetupFn) error
  NeedUpdate(comp *manifest.Component) bool
  ListContainers() ([]types.Container, error)
  ListImages() ([]types.ImageSummary, error)
  GetContainersByName(name string) (*types.Container, error)
  CopyFromContainer(cont *types.Container, src, dest string) error
  CopyToContainer(cont *types.Container, src_path, dest_path string) error

  FetchImage(comp *manifest.Component) error
  CleanupImage(cont *types.Container) error
  CleanupContainer(cont *types.Container) error
  StartContainer(comp *manifest.Component) error
  DeprecateComponent(comp *manifest.Component)
  BackupContainer(comp *manifest.Component) (*types.Container, error)
}

type IUpdater interface {
  SetupComponents(mani *manifest.UpdateManifest)
}
