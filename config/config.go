package config

import (
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	Inbounds  []Inbound  `json:"inbounds,omitempty"`  // 入站代理
	Outbounds []Outbound `json:"outbounds,omitempty"` // 出站代理
	Rules     []string   `json:"rules,omitempty"`     // 路由规则
}

type Inbound struct {
	Protocol string          `json:"protocol"` // 代理协议，可取值: socks, http, yeager
	Setting  json.RawMessage `json:"setting"`
}

type Outbound struct {
	Tag      string          `json:"tag"`      // 出站标记，用于路由规则指定出站代理
	Protocol string          `json:"protocol"` // 代理协议，可取值: socks, http, yeager
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
