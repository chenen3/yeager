package armin

type ClientConfig struct {
	Host      string          `json:"host"`
	Port      int             `json:"port"`
	UUID      string          `json:"uuid"`
	Transport string          `json:"transport"` // tls, grpc
	TLS       tlsClientConfig `json:"tls"`
}

type tlsClientConfig struct {
	ServerName string `json:"serverName"`
	Insecure   bool   `json:"insecure"` // (optional) developer only
}

type ServerConfig struct {
	Host      string          `json:"host"`
	Port      int             `json:"port"`
	UUID      string          `json:"uuid"`
	Transport string          `json:"transport"` // tls, grpc
	TLS       tlsServerConfig `json:"tls"`
	Fallback  fallback        `json:"fallback"` // (optional) if auth fail, fallback to HTTP server, such as nginx
}

type tlsServerConfig struct {
	CertFile     string `json:"certFile"`
	KeyFile      string `json:"keyFile"`
	certPEMBlock []byte
	keyPEMBlock  []byte
}

type fallback struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}