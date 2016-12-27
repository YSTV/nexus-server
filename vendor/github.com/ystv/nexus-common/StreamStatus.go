package nexus_common

type StreamStatus string

const (
	StreamStatusOnline StreamStatus = "ONLINE"
	StreamStatusTerminating StreamStatus = "TERMINATING" //
)

// StreamUpdate contains data about a currently running livestream
type StreamUpdate struct {
	StreamName string `json:"stream_name"`
	ClientAddress string `json:"client_address`
	Status StreamStatus `json:"status"`
}
