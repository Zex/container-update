package common

import (
  "strconv"
  "fmt"
  "sort"
  "strings"
  "errors"
  "os"
)

var (
  VERSION = os.Getenv("VERSION_DETAILS")
)

type VersionNumber struct {
  major int
  minor int
  patch int
  build int
}

func parse_version(ver string) (VersionNumber) {
  vs := strings.Split(ver, ".")
  eInvalid := errors.New(fmt.Sprintf("invalid version: %s", ver))

  if len(vs) < 4 { panic(eInvalid) }

  major, err := strconv.Atoi(vs[0])
  if err != nil { panic(eInvalid) }

  minor, err := strconv.Atoi(vs[1])
  if err != nil { panic(eInvalid) }

  patch, err := strconv.Atoi(vs[2])
  if err != nil { panic(eInvalid) }

  build, err := strconv.Atoi(vs[3])
  if err != nil { panic(eInvalid) }

  return VersionNumber {
    major: major,
    minor: minor,
    patch: patch,
    build: build,
  }
}

type Interface interface {
  String() string
}

func (ver VersionNumber) String() (string) {
  return fmt.Sprintf("%d.%d.%d.%d", ver.major, ver.minor, ver.patch, ver.build)//strconv.Itoa(ver.build))
}

type ByVer []VersionNumber
func (vers ByVer) Len() int { return len(vers) }
func (vers ByVer) Swap(i, j int) { vers[i], vers[j] = vers[j], vers[i] }
func (vers ByVer) Less(i, j int) bool {
  switch {
    case vers[i].major > vers[j].major:
      return false
    case vers[i].major < vers[j].major:
      return true
    case vers[i].minor > vers[j].minor:
      return false
    case vers[i].minor < vers[j].minor:
      return true
    case vers[i].patch > vers[j].patch:
      return false
    case vers[i].patch < vers[j].patch:
      return true
  }

  return vers[i].build < vers[j].build
}

func SortTags(tags []string) {
  vers := make(ByVer, len(tags))

  for i := range(tags) {
    vers[i] = parse_version(tags[i])
  }

  sort.Sort(vers)

  for i := range(tags) {
    tags[i] = vers[i].String()
  }
}
