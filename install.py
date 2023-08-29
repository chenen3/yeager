import os

def install():
    os.system("wget https://github.com/chenen3/yeager/releases/latest/download/yeager-linux-amd64.tar.gz")
    os.system("tar -xvf yeager-linux-amd64.tar.gz")
    os.system("cp yeager /usr/local/bin/yeager")
    if not os.path.exists("/usr/local/bin/yeager/server.json"):
        os.system("mkdir -p /usr/local/etc/yeager")
        os.chdir("/usr/local/etc/yeager")
        os.system("/usr/local/bin/yeager -genconf")

def setup_service():
    with open("/etc/systemd/system/yeager.service", "w") as f:
        f.write("""
    [Unit]
    Description=yeager
    Documentation=https://github.com/chenen3/yeager
    After=network.target

    [Service]
    ExecStart=/usr/local/bin/yeager -config /usr/local/etc/yeager/server.json
    TimeoutStopSec=5s
    LimitNOFILE=1048576
    # be able to bind privileged ports, e.g. 443
    AmbientCapabilities=CAP_NET_BIND_SERVICE

    [Install]
    WantedBy=multi-user.target
    """)
    os.system("systemctl daemon-reload")
    os.system("systemctl enable yeager")
    os.system("systemctl start yeager")
    
    os.system("sysctl -w net.core.default_qdisc=fq")
    os.system("sysctl -w net.ipv4.tcp_congestion_control=bbr")


if __name__ == "__main__":
    if os.geteuid() != 0:
        print("Please run as root")
        exit(1)
    install()
    setup_service()
    print("Installation finished, please update firewall and allow TCP port 57175")
