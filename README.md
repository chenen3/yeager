# yeager

This repository implements a proxy tool for speeding up the network.
I wrote it as a hobby and made it as simple and efficient as possible for personal use.

Features:
- tunnel over gRPC or QUIC, secure by mutual TLS
- rule-based routing
- SOCKS and HTTP proxy

Here is how the traffic flows:

```
browser -> [HTTP proxy -> yeager client] -> firewall -> [yeager server] <-> endpoints
```

## Install

### Binaries
Manually download the [release](https://github.com/chenen3/yeager/releases)
```sh
# assuming Linux, amd64 architecture
curl -LO https://github.com/chenen3/yeager/releases/latest/download/yeager-linux-amd64.tar.gz
tar -xzvf yeager-linux-amd64.tar.gz
mv yeager /usr/local/bin/
mkdir -p /usr/local/share/yeager
mv geosite.dat /usr/local/share/yeager/
```

### Docker
```sh
docker pull ghcr.io/chenen3/yeager
```

### Homebrew
```sh
brew tap chenen3/yeager
brew install yeager
```

### Source
```sh
go install github.com/chenen3/yeager@latest
```

## As remote server

### 1. Generate config

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

### 2. Run service

```sh
yeager -config /usr/local/etc/yeager/config.yaml
# if you prefer docker:
# docker run -d --restart=always --name yeager -v /usr/local/etc/yeager:/usr/local/etc/yeager -p 9001:9001 ghcr.io/chenen3/yeager
```

### 3. Update firewall
**Allow TCP port 9001**

## As local client

### 1. Configure

On remote host we have generated client config `usr/local/etc/yeager/client.yaml`, now copy it to local host as `/usr/local/etc/yeager/config.yaml`

### 2. Run service

For the binary:
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

### 3. Setup proxy
**setup HTTP proxy 127.0.0.1:8080 or SOCKS proxy 127.0.0.1:1080 on local host**.

That's all, good luck.

(For more details, see below...)

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
- `geosite,cn,direct` access directly if the domain name is located in mainland China. The geosite rule supports [domain-list-community](https://github.com/v2fly/domain-list-community)
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

- [trojan-gfw/trojan](https://github.com/trojan-gfw/trojan)
- [v2fly/v2ray-core](https://github.com/v2fly/v2ray-core)
- [v2fly/domain-list-community](https://github.com/v2fly/domain-list-community)
- [lucas-clemente/quic-go](https://github.com/lucas-clemente/quic-go)
