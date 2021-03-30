package yeager

import (
	"encoding/json"
	"io/ioutil"
)

type ClientConfig struct {
	Host string `json:"host"` // 必须填写域名
	Port int    `json:"port"`
	UUID string `json:"uuid"`
	// 控制客户端是否验证服务器的证书，仅供测试，不能在生产环境开启此选项
	InsecureSkipVerify bool `json:"insecureSkipVerify"`
}

type ServerConfig struct {
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	UUID     string    `json:"uuid"`
	CertFile string    `json:"certFile"`
	KeyFile  string    `json:"keyFile"`
	Fallback *fallback `json:"fallback"` // while auth fail, fallback to HTTP server, such as nginx

	// TODO: tidy
	certPEMBlock []byte
	keyPEMBlock  []byte
}

type fallback struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (s *ServerConfig) UnmarshalJSON(data []byte) error {
	// 结构体在反序列化时调用此 UnmarshalJSON 方法，如果此方法内部再对此结构体做反序列化，
	// 将陷入无尽递归，栈溢出。为了避免这个问题，给原始类型起一个别名，
	// 继承原始结构体字段，但不继承它的方法，把别名作为反序列化对象
	type alias ServerConfig
	a := new(alias)
	err := json.Unmarshal(data, a)
	if err != nil {
		return err
	}

	s.Host = a.Host
	s.Port = a.Port
	s.UUID = a.UUID
	s.Fallback = a.Fallback
	s.certPEMBlock, err = ioutil.ReadFile(a.CertFile)
	if err != nil {
		return err
	}
	s.keyPEMBlock, err = ioutil.ReadFile(a.KeyFile)
	return err
}
