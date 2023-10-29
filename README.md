# Yeager

A proxy that helps speed up the internet connection.

Features:
- tunneling over gRPC or HTTP2, secured by mutual TLS
- rule-based routing
- SOCKS and HTTP proxy

Here is how the traffic flows:

```
browser -> [HTTP proxy -> yeager client] -> firewall -> [yeager server] -> endpoints
```

## As remote server

1. install
    ```sh
    $ wget https://raw.githubusercontent.com/chenen3/yeager/master/install.py
    # Please run as root
    $ python3 install.py
    ```
    The script generates an client config file: `/usr/local/etc/yeager/client.json`
2. update firewall and allows TCP port 57175.

## As local client

1. download [here](https://github.com/chenen3/yeager/releases/latest)
2. copy the client.json to local device, run:
    ```sh
    $ yeager -config client.json
    ```
3. SOCKS proxy server address is 127.0.0.1:1080, HTTP proxy server address is 127.0.0.1:8080

See also [running daemon on macOS](TODO)

## use with Docker

build image:

```sh
$ docker build -t yeager .
```

remote server:

```sh
$ docker run --rm \
    --workdir /usr/local/etc/yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    yeager \
    /usr/local/bin/yeager -genconf

$ docker run -d --restart=always --name yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    -p 57175:57175 \
    yeager \
    /usr/local/bin/yeager -config /usr/local/etc/yeager/server.json
```

local client:
> **Note** copy `/usr/local/etc/yeager/client.json` from remote to local device
```sh
$ docker run -d \
    --restart=always \
    --network host \
    --name yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    yeager \
    /usr/local/bin/yeager -config /usr/local/etc/yeager/client.json
```

## Credit

- [grpc/grpc-go](https://github.com/grpc/grpc-go)
