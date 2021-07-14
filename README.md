# yeager

> Note: yeager is my personal training project, mostly learn from v2ray-core and xray-core, and do it in my way. If you are beginner looking for proxy tool, please consider [v2ray-core](https://github.com/v2fly/v2ray-core) or [Xray-core](https://github.com/XTLS/Xray-core) firstly, they are strong enough and having better community support.

yeager aims to bypass network restriction, has the basic features:

- socks5, http inbound proxy
- lightweight outbound proxy, secure transport via:
  - TLS
  - gRPC
- rule routing

## Install

Homebrew:

```
brew tap chenen3/yeager
brew install yeager
```

Docker:

```
docker pull en180706/yeager
```

## Configure

### Example for client side

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
                "uuid": "", // fill in UUID (uuidgen can help create one)
                "transport": "tls" // tls, grpc
            }
        }
    ],
    "rules": [
        "GEOIP,private,DIRECT",
        "GEOIP,cn,DIRECT",
        "GEOSITE,private,DIRECT",
        "GEOSITE,apple@cn,DIRECT",
        "GEOSITE,cn,DIRECT",
        "FINAL,PROXY"
    ]
}
```

The priority of rules is the order in config, the form could be `ruleType,value,outboundTag` and `FINAL,outboundTag`, for example:

- `DOMAIN,www.apple.com,DIRECT` matches if the domain of traffic is the given one
- `DOMAIN-SUFFIX,apple.com,DIRECT` matches if the domain of traffic has the suffix, AKA subdomain name
- `DOMAIN-KEYWORD,apple,DIRECT ` matches if the domain of traffic has the keyword
- `GEOSITE,cn,DIRECT` matches if the domain in [geosite](https://github.com/v2fly/domain-list-community/tree/master/data)
- `IP,127.0.0.1,DIRECT ` matches if the IP of traffic is the given one
- `GEOIP,cn,DIRECT` matches if the IP of traffic in [geoip](https://github.com/v2fly/geoip)
- `FINAL,PROXY` determine the behavior where would the traffic be send to if all above rule not match. The final rule must be the last rule in config.

Beside user specified outbound tag, there is two builtin:`DIRECT`, `REJECT`, for example:

- `GEOSITE,private,DIRECT` 
- `GEOSITE,category-ads,REJECT` 

### Example for server side

> Ensure the TLS certificate and key installed in directory `/usr/local/etc/yeager`. (If not, checkout Let's Encrypt)

Edit config file`/usr/local/etc/yeager/config.json`

```json
{
    "inbounds": [
        {
            "protocol": "armin",
            "setting": {
                "port": 443,
                "uuid": "", // fill in UUID (uuidgen can help create one)
                "transport": "tls", // tls, grpc
                "tls": {
                    "certFile": "/usr/local/etc/yeager/cert.pem", // replace with absolute path of certificate
                    "keyFile": "/usr/local/etc/yeager/key.pem", // replace with absolute path of key
                },
                "fallback": {
                    "host": "", // (optional) any other http server host (eg. nginx)
                    "port": 80 // (optional) any other http server port (eg. nginx)
                }
            }
        }
    ]
}
```

## Run

Homebrew:

`brew services start yeager`

Docker:

```
docker run \
	--name yeager \
	-d \
	--restart=always \
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	--network host \
	en180706/yeager
```

## Update

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

