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

Download from release:
```sh
wget https://github.com/chenen3/yeager/releases/latest/download/yeager-linux-amd64.tar.gz
tar -xzvf yeager-linux-amd64.tar.gz
./yeager -genconf -server server.json -client client.json
```

Here generates the a pair of client and server config, run with the server one:
```
./yeager -config server.json
```

Ensure the firewall allows TCP port 57175.

See also the install [script](https://raw.githubusercontent.com/chenen3/yeager/master/install.sh), 
which sets up the daemon and BBR(congestion control algorithm).

## As local client

Download [here](https://github.com/chenen3/yeager/releases/latest)

Remember the client config file generated before? Now copy it to the local machine.

Run with client.json:
```sh
yeager -config client.json
```

Then the HTTP proxy serve on 127.0.0.1:8080, SOCKS proxy serve on 127.0.0.1:1080

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
In case you prefer Docker over binary install, that's fine.

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
    yeager -genconf -server config.json -client client.json

docker run -d --restart=always --name yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    -p 57175:57175 \
    yeager
```

local client:

```sh
# copy `/usr/local/etc/yeager/client.json` from remote 
# to local machine as `/usr/local/etc/yeager/config.json`
docker run -d \
    --restart=always \
    --network host \
    --name yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    yeager
```

## Credit

- [ginuerzh/gost](https://github.com/ginuerzh/gost)
- [grpc/grpc-go](https://github.com/grpc/grpc-go)
- [lucas-clemente/quic-go](https://github.com/lucas-clemente/quic-go)
- [v2fly/domain-list-community](https://github.com/v2fly/domain-list-community)
- [refraction-networking/utls](https://github.com/refraction-networking/utls)
