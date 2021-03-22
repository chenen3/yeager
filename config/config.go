package config

import (
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	Inbound  Proto `json:"inbound,omitempty"`
	Outbound Proto `json:"outbound,omitempty"`
}

type Proto struct {
	Protocol string          `json:"protocol"` // 可取值为 socks, yeager, freedom
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
