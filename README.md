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
    `/usr/local/etc/yeager/client.json` is generated here, which will be used later.
2. update firewall and allows TCP port 57175.

## As local client

1. download [here](https://github.com/chenen3/yeager/releases/latest)
2. copy the client.json to local device, run:
    ```sh
    $ yeager -config client.json
    ```
3. SOCKS proxy server address is 127.0.0.1:1080, HTTP proxy server address is 127.0.0.1:8080

## Credit

- [grpc/grpc-go](https://github.com/grpc/grpc-go)
- [Jigsaw-Code/outline-sdk](https://github.com/Jigsaw-Code/outline-sdk)
