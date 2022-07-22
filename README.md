# yeager

This repository implements a proxy tool to help developers access restricted websites.
I wrote it as a hobby and made it as simple and efficient as possible for personal use.

Features:
- SOCKS and HTTP proxy
- lightweight tunnel proxy
  - transport over gRPC or QUIC
  - secure by mutual TLS
- Rule-based routing (supports [domain-list](https://github.com/v2fly/domain-list-community) from V2Ray)

## Get started

### As server on remote host

Generate config (self-signed certificates included):

```sh
mkdir -p /usr/local/etc/yeager
cd /usr/local/etc/yeager
docker run --rm \
    --workdir /usr/local/etc/yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    ghcr.io/chenen3/yeager \
    yeager -genconf
ln -s server.yaml config.yaml
```

the command generates two files:
- `/usr/local/etc/yeager/server.yaml` the server config
- `/usr/local/etc/yeager/client.yaml` the client config that should be **copyed to client device later**

Launch:

```sh
docker run -d \
    --name yeager \
    --restart=always \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    -p 9001:9001 \
    ghcr.io/chenen3/yeager
```

Update the firewall of remote host, **allow TCP port 9001**

### As client on local host

#### Install via homebrew (macOS only)

```sh
brew tap chenen3/yeager
brew install yeager
```

#### Install via docker

```sh
docker pull ghcr.io/chenen3/yeager
```

#### Configure

On remote host we have generated client config `usr/local/etc/yeager/client.yaml`, now copy it to local host as `/usr/local/etc/yeager/config.yaml`

#### Run via homebrew

```sh
brew services start yeager
```

#### Run via docker

```sh
docker run -d \
    --name yeager \
    --restart=always \
    --network host \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    ghcr.io/chenen3/yeager
```

#### Setup proxy
**setup SOCKS proxy 127.0.0.1:1080 or HTTP proxy 127.0.0.1:8080 on local host**.

That's all, good luck.

## Usage

### Generate config 
```sh
yeager -genconf
# generates a pair of server and client configuration files.
```

### Run service
```sh
yeager -config config.yaml
```

### Routing rule

Routing rule specifies where the incomming request goes to. It supports two forms:
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
