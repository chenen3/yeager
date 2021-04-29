package config

import (
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	Inbounds  []Proto  `json:"inbounds,omitempty"`  // 入站代理: socks, http, yeager
	Outbounds []Proto  `json:"outbounds,omitempty"` // 出站代理: yeager
	Rules     []string `json:"rules,omitempty"`
}

type Proto struct {
	Tag      string          `json:"tag"`      // 出站标记
	Protocol string          `json:"protocol"` // 代理协议，可取值为 socks, http, yeager
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
