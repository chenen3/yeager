package config

type Config struct {
	SOCKSListen   string         `yaml:"socksListen,omitempty"`
	HTTPListen    string         `yaml:"httpListen,omitempty"`
	TunnelListens []TunnelListen `yaml:"tunnelListens,omitempty"`
	TunnelClients []TunnelClient `yaml:"tunnelClients,omitempty"`
	Rules         []string       `yaml:"rules,omitempty"`
	// verbose logging
	Verbose bool `yaml:"verbose,omitempty"`
	// enable HTTP server for debugging and profiling, developers only
	Debug bool `yaml:"debug,omitempty"`
}

type TunnelType string

const (
	TunTCP  TunnelType = "tcp" // plain text
	TunGRPC TunnelType = "grpc"
	TunQUIC TunnelType = "quic"
	// deprecated. Infrequently used
	// TunTCP TunnelType = "tls"
)

type TunnelListen struct {
	Type   TunnelType `yaml:"type"`
	Listen string     `yaml:"listen"`
	// the TLS config is unavailable when the tunnel server is plain TCP
	CertFile string `yaml:"certFile,omitempty"`
	CertPEM  string `yaml:"certPEM,omitempty"`
	KeyFile  string `yaml:"keyFile,omitempty"`
	KeyPEM   string `yaml:"keyPEM,omitempty"`
	CAFile   string `yaml:"caFile,omitempty"`
	CAPEM    string `yaml:"caPEM,omitempty"`
}

type TunnelClient struct {
	Type    TunnelType `yaml:"type"`
	Policy  string     `yaml:"policy"`
	Address string     `yaml:"address"` // target server address

	// the TLS config is unavailable when the tunnel server is plain TCP
	CertFile string `yaml:"certFile,omitempty"`
	CertPEM  string `yaml:"certPEM,omitempty"`
	KeyFile  string `yaml:"keyFile,omitempty"`
	KeyPEM   string `yaml:"keyPEM,omitempty"`
	CAFile   string `yaml:"caFile,omitempty"`
	CAPEM    string `yaml:"caPEM,omitempty"`

	// optional
	ConnectionPoolSize int `yaml:"connectionPoolSize,omitempty"`
}
