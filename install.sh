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
User=ubuntu
Group=ubuntu
ExecStart=/usr/local/bin/yeager -config /usr/local/etc/yeager/config.json
TimeoutStopSec=5s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable yeager
systemctl start yeager

# use BBR congestion control
if ! lsmod | grep -q "bbr"; then
	echo "enable BBR congestion control..."
	echo net.core.default_qdisc=fq >> /etc/sysctl.conf
	echo net.ipv4.tcp_congestion_control=bbr >> /etc/sysctl.conf
	# increase the maximum buffer size for quic-go
	echo net.core.rmem_max=2500000 >> /etc/sysctl.conf
	sysctl -p
fi

echo "\nA few steps to do:"
echo "1. allows TCP port 57175"
echo "2. use /usr/local/etc/yeager/client.json to config the client"
