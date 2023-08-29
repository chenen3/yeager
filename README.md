# Yeager

A proxy that helps speed up the internet connection.

Features:
- tunneling over gRPC, QUIC or HTTP2, secured by mutual TLS
- rule-based routing
- SOCKS and HTTP proxy

Here is how the traffic flows:

```
browser -> [HTTP proxy -> yeager client] -> firewall -> [yeager server] -> endpoints
```

## As remote server

1. install
    ```sh
    wget https://raw.githubusercontent.com/chenen3/yeager/master/install.py
    # Please run as root
    python3 install.py
    ```
    The script generates an client config file: `/usr/local/etc/yeager/client.json`
2. update firewall and allows TCP port 57175.

## As local client

1. download [here](https://github.com/chenen3/yeager/releases/latest)
2. copy the client.json to local device, run:
    ```sh
    yeager -config client.json
    ```
3. SOCKS proxy server address is 127.0.0.1:1080, HTTP proxy server address is 127.0.0.1:8080

See also [running daemon on macOS](TODO)

<!-- TODO: move to wiki page -->
## Advance usage
### Routing rule

The routing rule specifies where the incoming traffic is sent to. It supports two forms:
- `type,value,policy`
- `final,policy`

The proxy policy is specified by config, also Yeager comes with two built-in proxy policies:

- `direct` means connecting directly, not through a tunnel server
- `reject` means connection rejected

For example:

- `ip-cidr,127.0.0.1/8,direct` access directly if IP matches
- `domain,www.apple.com,direct` access directly if the domain name matches
- `domain-suffix,apple.com,direct`access directly if the root domain name matches
- `domain-keyword,apple,direct` access directly if keyword matches
- `geosite,cn,direct` access directly if the domain name is located in CN.
    > **Note** 
    >
    > need to download the pre-defined domain list first:
    > ```sh
    > wget --output-document /usr/local/etc/yeager/geosite.dat \
    >   https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat
    > ```
- `final,proxy` access through the proxy server. If present, must be the last rule, by default is `final,direct`

### Docker

build image:

```sh
docker build -t yeager .
```

remote server:

```sh
docker run --rm \
    --workdir /usr/local/etc/yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    yeager \
    /usr/local/bin/yeager -genconf

docker run -d --restart=always --name yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    -p 57175:57175 \
    yeager \
    /usr/local/bin/yeager -config /usr/local/etc/yeager/server.json
```

local client:
> **Note** copy `/usr/local/etc/yeager/client.json` from remote to local device
```sh
docker run -d \
    --restart=always \
    --network host \
    --name yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    yeager \
    /usr/local/bin/yeager -config /usr/local/etc/yeager/client.json
```

## Credit

- [ginuerzh/gost](https://github.com/ginuerzh/gost)
- [grpc/grpc-go](https://github.com/grpc/grpc-go)
- [lucas-clemente/quic-go](https://github.com/lucas-clemente/quic-go)
- [v2fly/domain-list-community](https://github.com/v2fly/domain-list-community)
- [refraction-networking/utls](https://github.com/refraction-networking/utls)
