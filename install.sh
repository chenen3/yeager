#!/bin/bash
# This script deploys the yeager server.
# Tested in Ubuntu 22.04 LTS

# check if the script is running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run this script as root!"
    exit 1
fi

# install yeager
cd /tmp
wget https://github.com/chenen3/yeager/releases/latest/download/yeager-linux-amd64.tar.gz
tar -xzvf yeager-linux-amd64.tar.gz
cp yeager /usr/local/bin/yeager
mkdir -p /usr/local/etc/yeager
/usr/local/bin/yeager -genconf -srvconf /usr/local/etc/yeager/config.json \
	-cliconf /usr/local/etc/yeager/client.json

# run service
cat >> /etc/systemd/system/yeager.service << EOF
[Unit]
Description=yeager
Documentation=https://github.com/chenen3/yeager
After=network.target

[Service]
ExecStart=/usr/local/bin/yeager -config /usr/local/etc/yeager/config.json
TimeoutStopSec=5s
LimitNOFILE=1048576
# bind privileged ports, e.g. 443
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable yeager
systemctl start yeager

# use BBR congestion control
sudo sysctl -w net.core.default_qdisc=fq
sudo sysctl -w net.ipv4.tcp_congestion_control=bbr

echo "\nA few steps to do:"
echo "1. allows TCP port 57175"
echo "2. use /usr/local/etc/yeager/client.json to config the client"
