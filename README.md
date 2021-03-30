# yeager

A proxy aims to bypass network restriction.  Inspired by [v2ray](https://github.com/v2fly/v2ray-core), [trojan](https://github.com/trojan-gfw/trojan) and [trojan-go](https://github.com/p4gefau1t/trojan-go), for personal practising purpose, only implement the basic features: TCP based proxy and routing. Beside customize rule, the  router also support [domain-list-community](https://github.com/v2fly/domain-list-community/tree/master/data) and [geoip](https://github.com/v2fly/geoip)

## Installation

Please ensure you have docker installed.

For local machine, place the `config.json` in `/usr/local/etc/yeager`, then:

```sh
# assuming you want to expose ports 10800 and 10801 on host
docker run -d --restart=always --name yeager -p 10800:10800 -p 10801:10801 -v /usr/local/etc/yeager:/usr/local/etc/yeager en180706/yeager
```

For remote machine(eg. VPS), place the config files in `/usr/local/etc/yeager` , includes `config.json`, `your-certificate-file` and `your-key-file`, then:

```sh
# prepare your domain which resolve to this machine and apply domain certificate.(eg. let's encrypt)
# install nginx (or any other web server), nginx will listen on port 80
apt install nginx
systemctl enable nginx
systemctl start nginx
docker run -d --restart=always --name yeager -p 443:443 -v /usr/local/etc/yeager:/usr/local/etc/yeager en180706/yeager
```

## Configuration

Example for local machine:

```json
{
    "inbounds": [
        {
            "protocol": "socks",
            "setting": {
                "host": "127.0.0.1",
                "port": 10800
            }
        },
        {
            "protocol": "http",
            "setting": {
                "host": "127.0.0.1",
                "port": 10801
            }
        }
    ],
    "outbound": {
        "protocol": "yeager",
        "setting": {
            "host": "example.com",// replace with your domain name
            "port": 443,
            "uuid": "" // fill in UUID
        }
    },
    "rules": [
        "GEOSITE,private,DIRECT",
        "GEOSITE,apple@cn,DIRECT",
        "GEOSITE,cn,DIRECT",
        "GEOIP,private,DIRECT",
        "GEOIP,cn,DIRECT",
        "FINAL,PROXY"
    ]
}
```

The priority of rules is the order in config, the form could be `ruleType,value,policyType` and `FINAL,policyType`, for example:

- `DOMAIN,www.apple.com,DIRECT` matches if the domain of traffic is the given one
- `DOMAIN-SUFFIX,apple.com,DIRECT` matches if the domain of traffic has the suffix, AKA subdomain name
- `DOMAIN-KEYWORD,apple,DIRECT ` matches if the domain of traffic has the keyword
- `GEOSITE,cn,DIRECT` matches if the domain in [geosite](https://github.com/v2fly/domain-list-community/tree/master/data)
- `IP,127.0.0.1,DIRECT ` matches if the IP of traffic is the given one
- `GEOIP,cn,DIRECT` matches if the IP of traffic in [geoip](https://github.com/v2fly/geoip)
- `FINAL,PROXY` determine the behavior where would the traffic be send to if all above rule not match. The final rule must be the last rule in config.

The policyType is one of `DIRECT`, `REJECT` and  `PROXY`, for example:

- `GEOSITE,private,DIRECT` 
- `GEOSITE,category-ads,REJECT` 
- `GEOSITE,google,PROXY`

Example for remote machine(eg. VPS):

```json
{
    "inbounds": [
        {
            "protocol": "yeager",
            "setting": {
                "port": 443,
                "uuid": "", // fill in UUID
                "certFile": "/path/to/cert.pem", // replace with absolute path of certificate
                "keyFile": "/path/to/key.pem", // replace with absolute path of key
                "fallback": {
                    "host": "example.com", // if deploy via docker then replace with your domain name or IP
                    "port": 80
                }
            }
        }
    ]
}
```

The configuration file usually placed in path `/usr/local/etc/yeager/config.json.`

There is a simple command to generate UUID: `uuidgen`

