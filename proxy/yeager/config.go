package yeager

type ClientConfig struct {
	Host string `json:"host"` // 必须填写域名
	Port int    `json:"port"`
	UUID string `json:"uuid"`
	// 控制客户端是否验证服务器的证书，仅供测试，不能在生产环境开启此选项
	InsecureSkipVerify bool `json:"insecureSkipVerify"`
}

type ServerConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	UUID        string `json:"uuid"`
	CertFile    string `json:"certFile"`
	KeyFile     string `json:"keyFile"`
	FallbackUrl string `json:"fallbackUrl"` // while auth fail, fallback to HTTP server, such as nginx

	certPEMBlock []byte
	keyPEMBlock  []byte
}
