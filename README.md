# yeager

> yeager is my personal training project, mostly learn from v2ray-core, and do it in my way. If you are beginner looking for similar tool, please consider [v2ray-core](https://github.com/v2fly/v2ray-core) or [Xray-core](https://github.com/XTLS/Xray-core) firstly, which have more features and better community support

yeager aims to bypass network restrictions, supports features:
- lightweight proxy for outbound
    - transport by gRPC (or TCP)
    - secure by TLS
- SOCKS5 and HTTP proxy for inbound
- routing by rule

## Requirement
Server reachable from public Internet, with port 9000 exposed

## Usage

### As server

Generate certificate files:
```bash
mkdir -p /usr/local/etc/yeager
cd /usr/local/etc/yeager
docker run --rm \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    ghcr.io/chenen3/yeager:latest \
    yeager cert --host [server-public-ip]
```

Create config file `/usr/local/etc/yeager/config.json`
```json
{
    "inbounds": {
        "yeager": {
            "address": "0.0.0.0:9000",
            "transport": "grpc",
            "security": "tls-mutual",
            "mtls": {
                "certFile": "/usr/local/etc/yeager/server-cert.pem",
                "keyFile": "/usr/local/etc/yeager/server-key.pem",
                "clientCAFile": "/usr/local/etc/yeager/ca-cert.pem"
            }
        }
    }
}
```

Launch:
```bash
docker run -d \
	--name yeager \
	--restart=always \
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	-p 9000:9000 \
	ghcr.io/chenen3/yeager:latest
```

- start with configuration file

### As client

#### Install

- Via homebrew on MacOS 

```
brew tap chenen3/yeager
brew install yeager
```

- Via docker

`docker pull ghcr.io/chenen3/yeager:latest`

#### Configure
> ensure you have copy the certificate files to client device, and place in directory `/usr/local/etc/yeager`

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
            "address": "server-ip:9000", // replace server-ip
            "transport": "grpc",
            "security": "tls-mutual",
            "mtls": {
                "certFile": "/usr/local/etc/yeager/client-cert.pem",
                "keyFile": "/usr/local/etc/yeager/client-key.pem",
                "rootCAFile": "/usr/local/etc/yeager/ca-cert.pem"
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

#### Run

- Via homebrew on MacOS

`brew services start yeager`

- Via docker

```bash
docker run -d \
	--name yeager \
	--restart=always \
	--network host \
	-v /usr/local/etc/yeager:/usr/local/etc/yeager \
	ghcr.io/chenen3/yeager:latest
```

After running client side yeager, do not forget to **setup SOCKS5 or HTTP proxy for client device**. Good luck


## Advance usage

### Routing rule

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

### Upgrade

Via homebrew on macOS

```bash
brew update
brew upgrade yeager
brew services restart yeager
```

Via docker

```bash
docker pull ghcr.io/chenen3/yeager:latest
docker stop yeager
docker rm yeager
# then create the container again as described above
```

### Uninstall

via homebrew:

```bash
brew uninstall yeager
brew untap chenen3/yeager
```

via docker:

```bash
docker stop yeager
docker rm yeager
docker image rm ghcr.io/chenen3/yeager
```
