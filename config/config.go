package config

import (
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	Inbounds []Proto `json:"inbounds,omitempty"` // 入站代理: socks, http, yeager
	Outbound Proto   `json:"outbound,omitempty"` // 出站代理: yeager

	// 代理规则，请求流量按规则出现的顺序进行匹配，一旦命中则转发给规则指定的目标。
	// 留意最后的FINAL规则，表示当其他规则匹配失败时，流量应该往哪里发送，若未指定，则默认走代理
	// 示例:
	// TODO 规则细节未确定
	// 	DOMAIN,geosite:private,direct
	// 	DOMAIN,geosite:cn,direct
	// 	DOMAIN,geosite:apple,direct
	// 	IP,geoip:private,DIRECT
	// 	IP,geoip:cn,DIRECT
	// 	FINAL,proxy
	Rules []string `json:"rules,omitempty"`
}

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
