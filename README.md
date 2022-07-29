# yeager

This repository implements a proxy that helps speed up the internet connection.
I wrote it as a hobby and made it as simple and efficient as possible for personal use.

Features:
- SOCKS and HTTP proxy
- lightweight tunnel proxy
  - transport over gRPC or QUIC
  - secure by mutual TLS
- Rule-based routing (supports [domain-list-community](https://github.com/v2fly/domain-list-community))

## Get started

### 1. Install

Install the [pre-built binary](https://github.com/chenen3/yeager/releases)
```sh
# assuming on Linux, amd64 architecture
curl -LO https://github.com/chenen3/yeager/releases/latest/download/yeager-linux-amd64.tar.gz
tar -xzvf yeager-linux-amd64.tar.gz
mv yeager /usr/local/bin/
mkdir -p /usr/local/share/yeager
mv geosite.dat /usr/local/share/yeager/
```

Or install from Docker
```sh
docker pull ghcr.io/chenen3/yeager
```

Or install from Homebrew (macOS only)
```sh
brew tap chenen3/yeager
brew install yeager
```

### 2. As server on remote host

#### 2.1 Generate config

```sh
mkdir -p /usr/local/etc/yeager
cd /usr/local/etc/yeager
yeager -genconf
# if you prefer docker:
# docker run --rm --workdir /usr/local/etc/yeager -v /usr/local/etc/yeager:/usr/local/etc/yeager ghcr.io/chenen3/yeager yeager -genconf
ln -s server.yaml config.yaml
```

here generates a pair of config:
- `/usr/local/etc/yeager/server.yaml` the server config
- `/usr/local/etc/yeager/client.yaml` the client config that should be **copyed to client device later**

#### 2.2 Run service

```sh
yeager -config /usr/local/etc/yeager/config.yaml
# if you prefer docker:
# docker run -d --restart=always --name yeager -v /usr/local/etc/yeager:/usr/local/etc/yeager -p 9001:9001 ghcr.io/chenen3/yeager
```

#### 2.3 Update firewall
**Allow TCP port 9001**

### 3. As client on local host

#### 3.1 Configure

On remote host we have generated client config `usr/local/etc/yeager/client.yaml`, now copy it to local host as `/usr/local/etc/yeager/config.yaml`

#### 3.2 Run service

For the pre-built binary:
```sh
yeager -config /usr/local/etc/yeager/config.yaml
```

For Homebrew (macOS only):
```sh
brew services start yeager
```

For Docker:
```sh
docker run -d \
    --restart=always \
    --network host \
    --name yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    ghcr.io/chenen3/yeager
```

#### 3.3 Setup proxy
**setup SOCKS proxy 127.0.0.1:1080 or HTTP proxy 127.0.0.1:8080 on local host**.

That's all, good luck.

## Routing rule

Routing rule specifies where the incomming traffic be sent to. It supports two forms:
- `ruleType,value,outboundTag`
- `final,outboundTag`

The Outbound tag is specified by config, also yeager comes with two built-in outbound tags:

- `direct` means access directly, not through a proxy server
- `reject` means access denied

For example:

- `ip-cidr,127.0.0.1/8,direct` access directly if IP matches
- `domain,www.apple.com,direct` access directly if domain name matches
- `domain-suffix,apple.com,direct`access directly if root domain name matches
- `domain-keyword,apple,direct` access directly if keyword matches
- `geosite,cn,direct` access directly if the domain name is located in mainland China
- `final,proxy` access through the proxy server. If present, must be the last rule, by default is `final,direct`

## Uninstall

For the pre-built binary:

```sh
rm /usr/local/bin/yeager
rm /usr/local/share/yeager/geosite.dat
rmdir /usr/local/share/yeager
rm /usr/local/etc/yeager/*.yaml
rmdir /usr/local/etc/yeager
```

For Homebrew:

```sh
brew uninstall yeager
brew untap chenen3/yeager
```

For Docker:

```sh
docker stop yeager
docker rm yeager
docker image rm ghcr.io/chenen3/yeager
```

## Credit

- [trojan](https://github.com/trojan-gfw/trojan)
- [v2ray](https://github.com/v2fly/v2ray-core)
- [quic-go](https://github.com/lucas-clemente/quic-go)
