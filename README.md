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

1. server reachable from public Internet (e.g. Amazon Lightsail), update firewall:
   - expose port 443 which ACME protocol requires for HTTPS challenge
   - expose port 9000 which yeager proxy server listen on

2. domain name that pointed (A records) at the server

## Usage

### As server

- start with environment variables

```bash
mkdir -p /usr/local/etc/yeager
podman run -d \
	--name yeager \
	--restart always \
	-e YEAGER_TRANSPORT=grpc \
	-e YEAGER_UUID=example-UUID `#replace with UUID` \
	-e YEAGER_DOMAIN=example.com `#replace with domain name` \
	-e YEAGER_EMAIL=xxx@example.com `#replace with email address` \
	-p 443:443 \
	-p 9000:9000 \
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	ghcr.io/chenen3/yeager:latest
```

- start with configuration file

create config file `/usr/local/etc/yeager/config.json`
```json
{
    "inbounds": {
        "armin": {
            "address": "0.0.0.0:9000",
            "uuid": "example-UUID", // replace with UUID
            "transport": "grpc",
	    "acme":{
                "domain": "example.com", // replace with domain name
                "email": "xxx@example.com" // replace with email address
            }
        }
    }
}
```

then execute command:
```bash
podman run -d \
	--name yeager \
	--restart=always \
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	-p 443:443 \
	-p 9000:9000 \
	ghcr.io/chenen3/yeager:latest
```

### As client

#### Install

- Via homebrew on MacOS 

```
brew tap chenen3/yeager
brew install yeager
```

- Via podman on Linux distribution

`podman pull ghcr.io/chenen3/yeager:latest`

#### Configure

create config file `/usr/local/etc/yeager/config.json`

```json
{
    "inbounds": {
        "socks": {
            "address": "127.0.0.1:1080"
        },
        "http": {
            "address": "127.0.0.1:8080"
        }
    },
    "outbounds": [
        {
            "tag": "PROXY",
            "address": "example.com:9000", // replace with domain name
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

- Via homebrew on MacOS

`brew services start yeager`

- Via podman on Linux distribution

```bash
podman run -d \
	--name yeager \
	--restart=always \
	--network host \
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	ghcr.io/chenen3/yeager:latest
```

After running client side yeager, do not forget to **setup local device's SOCKS5 or HTTP proxy**. Good luck


## Advance usage

### Configuration explain

```json
{
    "inbounds": {
        "socks": {
            "address": "127.0.0.1:1080" // SOCKS5 proxy listening address
        },
        "http": {
            "address": "127.0.0.1:8080" // HTTP proxy listening address
        },
        "armin": {
            "address": "0.0.0.0:9000", // yeager proxy listening address
            "uuid": "51aef373-e1f7-4257-a45d-e75e65d712c4",
            "transport": "grpc", // tcp, tls, grpc
            "plaintext": false, // whether accept gRPC request in plaintext
            "acme":{
                "domain": "example.com",
                "email": "xxx@example.com",
                "stagingCA": false // use staging CA in testing, in case lock out
            },
            "certFile": "/path/to/certificate", // used when ACME config left blank
            "keyFile": "/path/to/key" // used when ACME config left blank
        }
    },
    "outbounds": [
        {
            "tag": "PROXY", // tag value must be unique in all outbounds
            "address": "example.com:9000", // correspond to inbound armin domain name
            "uuid": "51aef373-e1f7-4257-a45d-e75e65d712c4", // correspond to inbound armin UUID
            "transport": "grpc", // correspond to inbound armin transport
            "plaintext": false // whether send gRPC request in plaintext
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

Routing rule supports two forms:`ruleType,value,outboundTag` and `FINAL,outboundTag`.
Outbound tag specified by the user, and yeager also comes with two built-in outbound tags:
- `DIRECT` means sending traffic directly, do not pass by proxy
- `REJECT` means rejecting traffic and close the connection

rule example:
- `IP-CIDR,127.0.0.1/8,DIRECT ` matches if destination IP is in specified CIDR
- `DOMAIN,www.apple.com,DIRECT` matches if destination domain is the given one
- `DOMAIN-SUFFIX,apple.com,DIRECT` matches if destination domain has the suffix, AKA subdomain name
- `DOMAIN-KEYWORD,apple,DIRECT ` matches if destination domain has the keyword
- `GEOSITE,cn,DIRECT` matches if destination domain is in [geosite](https://github.com/v2fly/domain-list-community/tree/master/data)
- `FINAL,PROXY` determine where the traffic be send to while all above rules not match. It must be the last rule, by default is `FINAL,DIRECT`


### Manually run yeager

1. download the latest [release](https://github.com/chenen3/yeager/releases)
2. create config file as explained
3. run command: `YEAGER_ASSET_DIR=. yeager -config config.json`

### Transport via TLS

If the network between local device and remote server is not good enough, please use the TLS transport feature, simply set `transport` field  to `tls` in both server and client config

### Transport in plaintext

**Please do not use plaintext unless you know what you are doing.**

In some situations we want reverse proxy or load balancing yeager server, yeager works with API gateway (e.g. nginx) which terminates TLS. Update yeager server config:
- while transport via tcp, set `transport` field to `tcp`
- while transport via gRPC, set `plaintext` fields to `true`

### Upgrade

Via homebrew on macOS

```bash
brew update
brew upgrade yeager
brew services restart yeager
```

Via podman on Linux distribution

```bash
podman pull ghcr.io/chenen3/yeager:latest
podman stop yeager
podman rm yeager
# then run the container again
```

### Uninstall

via homebrew:

```bash
brew uninstall yeager
brew untap chenen3/yeager
```

via podman:

```bash
podman container stop yeager
podman container rm yeager
podman image rm ghcr.io/chenen3/yeager
```
