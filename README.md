# yeager

> Note: yeager is my personal training project, mostly learn from v2ray-core and xray-core, and do it in my way. If you are beginner looking for proxy tool, please consider [v2ray-core](https://github.com/v2fly/v2ray-core) or [Xray-core](https://github.com/XTLS/Xray-core) firstly, they are strong enough and having better community support.

yeager aims to bypass network restriction, supports features:

- socks5, http inbound proxy
- lightweight outbound proxy, secure transport via:
  - TLS
  - gRPC
- rule routing

## Prerequisite
1. virtual private server (VPS) abroad; VPS's firewall allow inbound traffic from 80 and 443 port.

2. domain name; add an "A record" to the domain name, pointing to the VPS public IP

3. issue TLS certificate; CertBot or acme.sh can help.

## Server side

### Install

#### Docker

```
docker pull en180706/yeager
```

#### Manual

checkout the latest [release](https://github.com/chenen3/yeager/releases).

### Configure

Edit file `/usr/local/etc/yeager/config.json`

```json
{
    "inbounds": [
        {
            "protocol": "armin",
            "setting": {
                "port": 443,
                "uuid": "", // fill in UUID (command uuidgen to create one)
                "transport": "grpc", // grpc or tls
                "tls": {
                    "certFile": "/usr/local/etc/yeager/fullchain.pem", // install the TLS certificate to this path
                    "keyFile": "/usr/local/etc/yeager/key.pem" // install the TLS key to this path
                }
            }
        }
    ]
}
```

### Run

#### Docker

```
docker run \
	--name yeager \
	-d \
	--restart=always \
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	--network host \
	en180706/yeager
```

#### Manual

`yeager -config /usr/local/etc/yeager/config.json`

## Client side

### Install

#### MacOS using Homebrew

```
brew tap chenen3/yeager
brew install yeager
```

#### Linux distro Using Docker

`docker pull en180706/yeager`

#### Manual

checkout the latest [release](https://github.com/chenen3/yeager/releases).

### Configure

Edit config file`/usr/local/etc/yeager/config.json`

```json
{
    "inbounds": [
        {
            "protocol": "socks",
            "setting": {
                "host": "127.0.0.1",
                "port": 10800
            }
        },
        {
            "protocol": "http",
            "setting": {
                "host": "127.0.0.1",
                "port": 10801
            }
        }
    ],
    "outbounds": [
        {
            "tag": "PROXY",
            "protocol": "armin",
            "setting": {
                "host": "example.com", // replace with domain name
                "port": 443,
                "uuid": "", // fill in UUID
                "transport": "grpc" // grpc or tls
            }
        }
    ],
    "rules": [
		"IP-CIDR,127.0.0.1/8,DIRECT",
		"IP-CIDR,192.168.0.0/16,DIRECT",
		"GEOSITE,private,DIRECT",
		"GEOSITE,google,PROXY",
		"GEOSITE,twitter,PROXY",
		"GEOSITE,cn,DIRECT",
		"GEOSITE,apple@cn,DIRECT",
		"FINAL,PROXY"
    ]
}
```

The priority of rules is the order in config, the form could be `ruleType,value,outboundTag` and `FINAL,outboundTag`, for example:

- `IP-CIDR,127.0.0.1/8,DIRECT ` matches if the traffic IP is in specified CIDR
- `DOMAIN,www.apple.com,DIRECT` matches if the traffic domain is the given one
- `DOMAIN-SUFFIX,apple.com,DIRECT` matches if the traffic domain has the suffix, AKA subdomain name
- `DOMAIN-KEYWORD,apple,DIRECT ` matches if the traffic domain has the keyword
- `GEOSITE,cn,DIRECT` matches if the traffic domain is in [geosite](https://github.com/v2fly/domain-list-community/tree/master/data)
- `FINAL,PROXY` determine the behavior where would the traffic be send to while all above rule not match, it must be the last rule in config. The default final rule is `FINAL,DIRECT`

Beside user specified outbound tag, there are two builtin: `DIRECT`, `REJECT`, for example:

- `GEOSITE,private,DIRECT` 
- `GEOSITE,category-ads,REJECT` 

### Run

> After running client side yeager, do not forget to config the local device's SOCKS5 or HTTP proxy. Good luck

#### Homebrew

`brew services start yeager`

#### Docker

```
docker run \
	--name yeager \
	-d \
	--restart=always \
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	--network host \
	en180706/yeager
```

#### Manual

`yeager -config /usr/local/etc/yeager/config.json`

## Upgrade

Homebrew:

```
brew update
brew upgrade yeager
brew services restart yeager
```

Docker:

```
docker pull en180706/yeager
docker stop yeager
docker rm yeager
docker run \
	--name yeager \
	-d \
	--restart=always \
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	--network host \
	en180706/yeager
```

## Uninstall

Homebrew:

```
brew uninstall yeager
brew untap chenen3/yeager
```

Docker:

```
docker stop yeager
docker rm yeager
docker rmi en180706/yeager
```

