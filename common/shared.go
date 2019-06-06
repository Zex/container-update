package common

const (
  TopicUpdateManifest = "update_manifest"
  TopicHeartbeat = "heartbeat"
  TopicEvent = "event"
)

type Publisher interface {
  PublishEvent(data []byte) error
  PublishHeartbeat(data []byte) error
}
