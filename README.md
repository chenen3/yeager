# yeager

> Note: yeager is my personal training project, mostly learn from v2ray-core and xray-core, and do it in my way. If you are beginner looking for proxy tool, please consider [v2ray-core](https://github.com/v2fly/v2ray-core) or [Xray-core](https://github.com/XTLS/Xray-core) firstly, they are strong enough and having better community support.

yeager aims to bypass network restriction, supports features:

- SOCKS5 proxy for inbound usage
- HTTP proxy for inbound usage
- lightweight outbound proxy, secure transport via:
  - TLS
  - gRPC
- rule routing

## Requirements

1. server reachable from public Internet (e.g. Amazon Lightsail VPS)
   - expose port 443 for automate certificate management
   - expose the port that yeager proxy server listen on

2. domain name that pointed (A records) at the server

## Quick start

### Server side

run yeager service via docker cli

```bash
docker run -d \
	--name yeager \
	--restart always \
	-e ARMIN_ADDRESS=127.0.0.1:9000
	-e ARMIN_UUID=$(uuidgen)
	-e ARMIN_TRANSPORT=grpc
	-e ARMIN_DOMAIN=example.com `#replace with your domain name`
	-e $XDG_DATA_HOME=/usr/local/etc/yeager
	-p 443:443
	-p 9000:9000
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	ghcr.io/chenen3/yeager:latest
```

log the auto-generated UUID, it will be used as client config, run command:

`docker logs yeager 2>&1 | grep uuid | tail -n 1`

###Client side

####Install

- Via homebrew on MacOS 

```
brew tap chenen3/yeager
brew install yeager
```

- Via docker on Linux distro

`docker pull en180706/yeager`

##### Configure

create config file: `/usr/local/etc/yeager/config.json`

```json
{
    "inbounds": {
        "socks": {
            "address": "127.0.0.1:10800"
        },
        "http": {
            "address": "127.0.0.1:10801"
        }
    },
    "outbounds": [
        {
            "tag": "PROXY",
            "address": "example.com:443", // replace with domain name
            "uuid": "example-uuid", // replace with UUID
            "transport": "grpc"
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

#### Run

> After running client side yeager, do not forget to config the local device's SOCKS5 or HTTP proxy. Good luck

- Via homebrew on MacOS

`brew services start yeager`

- Via docker on Linux distro

```bash
docker run -d \
	--name yeager \
	--restart=always \
	--network host \
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	en180706/yeager
```

## Upgrade

Via homebrew on macOS

```bash
brew update
brew upgrade yeager
brew services restart yeager
```

Via docker on Linux distro (or podman)

```bash
docker pull en180706/yeager
docker stop yeager
docker rm yeager
docker run -d \
	--name yeager \
	--restart=always \
	--network host \
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	en180706/yeager
```

## Uninstall

via homebrew:

```bash
brew uninstall yeager
brew untap chenen3/yeager
```

via docker:

```bash
docker container stop yeager
docker container rm yeager
docker image rm en180706/yeager
```

##Advance usage

### Configuration explain

```json
{
    "inbounds": {
        "socks": {
            "address": ":10810" // SOCKS5 proxy listening address
        },
        "http": {
            "address": ":10811" // HTTP proxy listening address
        },
        "armin": {
            "address": ":10812", // yeager proxy listening address
            "uuid": "51aef373-e1f7-4257-a45d-e75e65d712c4",
            "transport": "grpc", // tcp, tls, grpc
            "plaintext": false // whether accept gRPC request in plaintext
        }
    },
    "outbounds": [
        {
            "tag": "PROXY", // tag value must be unique in all outbounds
            "address": "127.0.0.1:10812", // correspond to inbound armin address
            "uuid": "51aef373-e1f7-4257-a45d-e75e65d712c4", // correspond to inbound armin UUID
            "transport": "grpc", // correspond to inbound armin transport
            "plaintext": false, // whether send gRPC request in plaintext
            "acme":{
                "domain": "example.com", // domain name
                "email": "mail.example.com", // optional email address
                "stagingCA": false // use staging CA in testing, in case lock out
            },
            "certFile": "/path/to/certificate", // used when ACME config left blank
            "keyFile": "/path/to/key" // used when ACME config left blank
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

Routing rule supports two forms:`ruleType,value,outboundTag` and `FINAL,outboundTag`, for example:

- `IP-CIDR,127.0.0.1/8,DIRECT ` matches if the traffic IP is in specified CIDR
- `DOMAIN,www.apple.com,DIRECT` matches if the traffic domain is the given one
- `DOMAIN-SUFFIX,apple.com,DIRECT` matches if the traffic domain has the suffix, AKA subdomain name
- `DOMAIN-KEYWORD,apple,DIRECT ` matches if the traffic domain has the keyword
- `GEOSITE,cn,DIRECT` matches if the traffic domain is in [geosite](https://github.com/v2fly/domain-list-community/tree/master/data)
- `FINAL,PROXY` determine the behavior where would the traffic be send to while all above rule not match, it must be the last rule in config. The default final rule is `FINAL,DIRECT`

In addition to the outbound tag specified by the user, yeager also comes with two built-in tags:

- `DIRECT` means sending traffic directly, do not pass by proxy
- `REJECT` means rejecting traffic and close the connection

### Manaully run yeager

1. download the latest [release](https://github.com/chenen3/yeager/releases)
2. create config file as explained
3. run command: `YEAGER_ASSET_DIR=. yeager -config config.json`

### Transport via TLS

If the network between local device and remote server is not good enough, please use the TLS transport feature, simply set `transport` field  to `tls` in both server and client config

### Transport in plaintext

**Please do not use plaintext unless you know what you are doing.**

In some situations we want reverse proxy or load balancing yeager server, yeager works with API gateway (e.g. nginx) which terminates TLS. Update yeager server config:
- while transport via tcp, set `transport` field to `tcp`
- while transport via gRPC, set `plaintex` fields to `true`