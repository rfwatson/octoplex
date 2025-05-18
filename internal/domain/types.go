package domain

import (
	"slices"
	"time"
)

// AppState holds application state.
type AppState struct {
	Source       Source
	Destinations []Destination
	BuildInfo    BuildInfo
}

// Clone performs a deep copy of AppState.
func (s *AppState) Clone() AppState {
	return AppState{
		Source:       s.Source,
		Destinations: slices.Clone(s.Destinations),
		BuildInfo:    s.BuildInfo,
	}
}

// BuildInfo holds information about the build.
type BuildInfo struct {
	GoVersion string
	Version   string
	Commit    string
	Date      string
}

// Source represents the source, currently always the mediaserver.
type Source struct {
	Container     Container
	Live          bool
	LiveChangedAt time.Time
	Tracks        []string
	RTMPURL       string
	RTMPSURL      string
	ExitReason    string
}

// DestinationStatus reflects the high-level status of a single destination.
type DestinationStatus int

const (
	DestinationStatusOffAir DestinationStatus = iota
	DestinationStatusStarting
	DestinationStatusLive
)

// Destination is a single destination.
type Destination struct {
	Container Container
	Status    DestinationStatus
	Name      string
	URL       string
}

// NetAddr holds a network address.
type NetAddr struct {
	IP   string
	Port int
}

// IsZero returns true if the NetAddr is zero value.
func (n NetAddr) IsZero() bool {
	return n.IP == "" && n.Port == 0
}

// KeyPairs holds TLS key pairs for the server.
type KeyPairs struct {
	Internal KeyPair // valid for localhost and any user-provided host
	Custom   KeyPair // valid only for any user-provided host, may be zero value
}

// External returns the external key pair, which is the custom key pair if it
// exists or the internal key pair otherwise.
func (k KeyPairs) External() KeyPair {
	if k.Custom.IsZero() {
		return k.Internal
	}

	return k.Custom
}

// KeyPair holds a TLS key pair.
type KeyPair struct {
	Cert, Key []byte
}

// IsZero returns true if the KeyPair is zero value.
func (k KeyPair) IsZero() bool {
	return k.Cert == nil && k.Key == nil
}

// Container status strings.
//
// TODO: refactor to strictly reflect Docker status strings.
const (
	ContainerStatusPulling    = "pulling" // Does not correspond to a Docker status.
	ContainerStatusCreated    = "created"
	ContainerStatusRunning    = "running"
	ContainerStatusPaused     = "paused"
	ContainerStatusRestarting = "restarting"
	ContainerStatusRemoving   = "removing"
	ContainerStatusExited     = "exited"
	ContainerStatusDead       = "dead"
)

// Container represents the current state of an individual container.
type Container struct {
	ID               string
	Status           string
	HealthState      string
	CPUPercent       float64
	MemoryUsageBytes uint64
	RxRate           int
	TxRate           int
	RxSince          time.Time
	ImageName        string
	PullStatus       string // PullStatus is the status of the image pull.
	PullProgress     string // PullProgress is the "progress string" of the image pull.
	PullPercent      int    // PullPercent is the percentage of the image that has been pulled.
	RestartCount     int
	ExitCode         *int
	Err              error // Err is set if any error was received from the container client.
}
