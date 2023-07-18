# Yeager

A proxy that helps speed up the internet connection.

Features:
- tunneling over HTTP2, gRPC or QUIC, secured by mutual TLS
- rule-based routing
- SOCKS and HTTP proxy

Here is how the traffic flows:

```
browser -> [HTTP proxy -> yeager client] -> firewall -> [yeager server] -> endpoints
```

## As remote server

1. deploy
    ```sh
    wget https://raw.githubusercontent.com/chenen3/yeager/master/install.sh
    sudo bash install.sh
    ```
2. the script will generate a client config file at `/usr/local/etc/yeager/client.json`

3. update the firewall, allow TCP port 57175

## As local client

1. Install

    Download the release manually, or via homebrew:
    ```sh
    brew tap chenen3/yeager
    brew install yeager
    ```

2. Configure

    Remember the client config file generated before? Copy it to the local 
    machine as `/usr/local/etc/yeager/config.json`

3. Run service

    For Homebrew:
    ```sh
    brew services start yeager
    ```

    For the binary:
    ```sh
    yeager -config /usr/local/etc/yeager/config.json
    ```

4. Setup proxy
    - setting HTTP and HTTPS proxy to localhost:8080
    - setting SOCKS proxy to localhost:1080

So far, most needs should be met, good luck.

(For more details, see below...)

## Routing rule
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

## Uninstall

For the pre-built binary:

```sh
rm /usr/local/bin/yeager
rm /usr/local/etc/yeager/config.json
# In case you have downloaded the pre-defined domain list:
#   rm /usr/local/etc/yeager/geosite.dat
rmdir /usr/local/etc/yeager
```

For Homebrew:

```sh
brew uninstall yeager
brew untap chenen3/yeager
```

## Docker
In case you prefer Docker over binary installation, here is how to do it.

As a remote server:

```sh
docker run --rm \
    --workdir /usr/local/etc/yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    ghcr.io/chenen3/yeager \
    yeager -genconf -srvconf config.json -cliconf client.json

docker run -d --restart=always --name yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    -p 57175:57175 \
    ghcr.io/chenen3/yeager
```

As a local client:

```sh
# copy `/usr/local/etc/yeager/client.json` from remote 
# to local machine as `/usr/local/etc/yeager/config.json`
docker run -d \
    --restart=always \
    --network host \
    --name yeager \
    -v /usr/local/etc/yeager:/usr/local/etc/yeager \
    ghcr.io/chenen3/yeager
```

Uninstall

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
