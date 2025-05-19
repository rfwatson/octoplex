package mediaserver

// Config represents the MediaMTX configuration file.
type Config struct {
	LogLevel          string          `yaml:"logLevel,omitempty"`
	LogDestinations   []string        `yaml:"logDestinations,omitempty"`
	ReadTimeout       string          `yaml:"readTimeout,omitempty"`
	WriteTimeout      string          `yaml:"writeTimeout,omitempty"`
	WriteQueueSize    int             `yaml:"writeQueueSize,omitempty"`
	UDPMaxPayloadSize int             `yaml:"udpMaxPayloadSize,omitempty"`
	AuthMethod        string          `yaml:"authMethod,omitempty"`
	AuthInternalUsers []User          `yaml:"authInternalUsers,omitempty"`
	Metrics           bool            `yaml:"metrics,omitempty"`
	MetricsAddress    string          `yaml:"metricsAddress,omitempty"`
	API               bool            `yaml:"api,omitempty"`
	APIAddr           string          `yaml:"apiAddress,omitempty"`
	APIEncryption     bool            `yaml:"apiEncryption,omitempty"`
	APIServerCert     string          `yaml:"apiServerCert,omitempty"`
	APIServerKey      string          `yaml:"apiServerKey,omitempty"`
	RTMP              bool            `yaml:"rtmp"`
	RTMPEncryption    string          `yaml:"rtmpEncryption,omitempty"`
	RTMPAddress       string          `yaml:"rtmpAddress,omitempty"`
	RTMPSAddress      string          `yaml:"rtmpsAddress,omitempty"`
	RTMPServerCert    string          `yaml:"rtmpServerCert,omitempty"`
	RTMPServerKey     string          `yaml:"rtmpServerKey,omitempty"`
	HLS               bool            `yaml:"hls"`
	RTSP              bool            `yaml:"rtsp"`
	WebRTC            bool            `yaml:"webrtc"`
	SRT               bool            `yaml:"srt"`
	Paths             map[string]Path `yaml:"paths,omitempty"`
}

// Path represents a path configuration in MediaMTX.
type Path struct {
	Source string `yaml:"source,omitempty"`
}

// UserPermission represents a user permission in MediaMTX.
type UserPermission struct {
	Action string `yaml:"action,omitempty"`
}

// User represents a user configuration in MediaMTX.
type User struct {
	User        string           `yaml:"user,omitempty"`
	Pass        string           `yaml:"pass,omitempty"`
	IPs         []string         `yaml:"ips,omitempty"`
	Permissions []UserPermission `yaml:"permissions,omitempty"`
}
