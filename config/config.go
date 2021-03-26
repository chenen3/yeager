package config

import (
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	Inbounds []Proto  `json:"inbounds,omitempty"` // 入站代理: socks, http, yeager
	Outbound Proto    `json:"outbound,omitempty"` // 出站代理: yeager
	Rules    []string `json:"rules,omitempty"`
}

// TODO 精简配置结构，protocol应该与host,port等字段在同一个段落
type Proto struct {
	Protocol string          `json:"protocol"` // 可取值为 socks, http, yeager
	Setting  json.RawMessage `json:"setting"`
}

func Load(filename string) (Config, error) {
	bs, err := ioutil.ReadFile(filename)
	if err != nil {
		return Config{}, err
	}

	var conf Config
	err = json.Unmarshal(bs, &conf)
	return conf, err
}
