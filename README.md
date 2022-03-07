# yeager

Yeager is a tool for bypassing network restrictions. I learn the idea from Trojan and V2ray, then do it in my way, just a hobby. Supporting the following features:

- SOCKS5 and HTTP/HTTPS proxy
- lightweight tunnel proxy
  - transport over gRPC or QUIC
  - security via mutual TLS
- Rule-based routing

## Prerequisites

- The server is reachable from the public Internet and exposes TCP port 9000
- Docker

## Usage

### As server

Generate certificate files:

```sh
mkdir -p /usr/local/etc/yeager
cd /usr/local/etc/yeager
docker run --rm \
    --workdir /usr/local/etc/yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    ghcr.io/chenen3/yeager \
    yeager cert --host [server-public-ip]
```

Create config file `/usr/local/etc/yeager/config.json`

```json
{
    "inbounds": {
        "yeager": {
            "listen": "0.0.0.0:9000",
            "transport": "grpc",
            "mtls": {
                "certFile": "/usr/local/etc/yeager/server-cert.pem",
                "keyFile": "/usr/local/etc/yeager/server-key.pem",
                "caFile": "/usr/local/etc/yeager/ca-cert.pem"
            }
        }
    }
}
```

Launch:

```sh
docker run -d \
    --name yeager \
    --restart=always \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    -p 9000:9000 \
    ghcr.io/chenen3/yeager
```

### As client

#### Install

- Via homebrew (macOS only)

```sh
brew tap chenen3/yeager
brew install yeager
```

- Via docker

`docker pull ghcr.io/chenen3/yeager`

#### Configure

> At server side we have generated certificate files: client-cert.pem, client-key.pem, ca-cert.pem. Ensure you have copy these files to client device, and place in directory `/usr/local/etc/yeager`

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
            "tag": "proxy",
            "address": "example.server.ip:9000", // replace example server IP
            "transport": "grpc",
            "mtls": {
                "certFile": "/usr/local/etc/yeager/client-cert.pem",
                "keyFile": "/usr/local/etc/yeager/client-key.pem",
                "caFile": "/usr/local/etc/yeager/ca-cert.pem"
            }
        }
    ],
    "rules": [
        "ip-cidr,127.0.0.1/8,direct",
        "ip-cidr,192.168.0.0/16,direct",
        "geosite,private,direct",
        "geosite,google,proxy",
        "geosite,twitter,proxy",
        "geosite,cn,direct",
        "geosite,apple@cn,direct",
        "final,proxy"
    ]
}
```

#### Run

- Via homebrew on macOS

```sh
brew services start yeager
```

- Via docker

```sh
docker run -d \
    --name yeager \
    --restart=always \
    --network host \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    ghcr.io/chenen3/yeager
```

After running client side yeager, do not forget to **setup SOCKS5 or HTTP proxy for client device**. Good luck.

## Advance usage

### Routing rule

Routing rule specifies where the incomming request goes to. It supports two forms:
- `ruleType,value,outboundTag`
- `final,outboundTag`

The Outbound tag is specified by config, also yeager comes with two built-in outbound tags:

- `direct` means sending traffic directly, do not pass through proxy
- `reject` means rejecting traffic and close the connection

rule example:

- `ip-cidr,127.0.0.1/8,direct` matches if destination IP is in specified CIDR
- `domain,www.apple.com,direct` matches if destination domain is the given one
- `domain-suffix,apple.com,direct` matches if destination domain has the suffix, AKA subdomain name
- `domain-keyword,apple,direct` matches if destination domain has the keyword
- `geosite,cn,direct` matches if destination domain is in [geosite](https://github.com/v2fly/domain-list-community/tree/master/data)
- `final,proxy` determine where the traffic be send to while all above rules not match. It must be the last rule, by default is `final,direct`

### Upgrade

Via homebrew on macOS

```sh
brew update
brew upgrade yeager
brew services restart yeager
```

Via docker

```sh
docker pull ghcr.io/chenen3/yeager
docker stop yeager
docker rm yeager
# then create the container again as described above
```

### Uninstall

via homebrew:

```sh
brew uninstall yeager
brew untap chenen3/yeager
```

via docker:

```sh
docker stop yeager
docker rm yeager
docker image rm ghcr.io/chenen3/yeager
```

## Credit

- [trojan](https://github.com/trojan-gfw/trojan)
- [v2ray](https://github.com/v2fly/v2ray-core)
- [quic-go](https://github.com/lucas-clemente/quic-go)
