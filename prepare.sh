#!/bin/bash

# 此脚本为 yeager 服务端代理准备生产环境，意在简化部署流程，
# 自动开启BBR拥塞控制算法、申请TLS证书、安装docker

install_bbr() {
    echo "启用BBR ..."
    has_bbr=$(lsmod | grep bbr)
    if [ ! -n "$has_bbr" ] ;then
        echo net.core.default_qdisc=fq >> /etc/sysctl.conf
        echo net.ipv4.tcp_congestion_control=bbr >> /etc/sysctl.conf
        sysctl -p
    fi
}

DIR_ETC=/usr/local/etc/yeager
TLS_KEY=${DIR_ETC}/key.pem
TLS_CERT=${DIR_ETC}/fullchain.pem

create_cert() {
    if ! [ -n "$(command -v acme.sh)" ]; then
        echo "安装acme.sh ..."
        curl  https://get.acme.sh | sh
        ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt
    fi

    if [ ! -d $DIR_ETC ] ; then
        mkdir -p $DIR_ETC
    fi

    read -p "请输入指向本机IP的域名: " domain
    has_cert=$(~/.acme.sh/acme.sh --list|grep $$domain)
    if [ -n "$has_cert" ] ;then
        echo "本地已有该域名的TLS证书，无需申请"
        return
    fi

    echo "申请 TLS 证书 ..."
    ~/.acme.sh/acme.sh --issue -d $domain --standalone --keylength ec-256
    ~/.acme.sh/acme.sh --install-cert -d $domain --ecc \
    --key-file       $TLS_KEY  \
    --fullchain-file $TLS_CERT
}

install_docker() {
    echo "安装docker ..."
    if [ -x "$(command -v docker)" ]; then
        return 0
    fi

    sudo apt-get update
    sudo apt-get install -y apt-transport-https ca-certificates curl gnupg lsb-release
    curl -fsSL https://download.docker.com/linux/debian/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
    echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/debian $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
    sudo apt-get update
    sudo apt-get install -y docker-ce docker-ce-cli containerd.io
}

install_yeager(){
    if [ ! -x "$(command -v docker)" ]; then
        echo "docker not installed"
        return 1
    fi

    exists=$(docker ps --format "{{.Names}}" | grep yeager)
    if [ -n exists ]; then
        echo "container yeager already exists"
        return
    fi

    uuid=$(uuidgen)
    read -p "请输入监听端口号(默认443)：" port 
    if [ ! -n port ] ; then
        port=443
    fi
    read -p "请输入传输协议 [tls/grpc]: " trans
    if [ ! -n trans ] ; then
        trans="tls"
    fi

    # TODO: 目前不支持以命令行参数启动，待定
    docker run \
        --name yeager \
        -d \
        --restart=always \
        -v ${DIR_ETC}:/usr/local/etc/yeager \
        --network host \
        en180706/yeager \
        yeager -uuid $uuid -port $port -transport $trans -key $TLS_KEY -cert $TLS_CERT
    echo "generate yeager server side proxy config file ${DIR_ETC}/config.json :"
    cat ${DIR_ETC}/config.json
}

main() {
    if [ $(id -u) != 0 ]; then
        echo "请使用root权限运行"
        return 1
    fi

    install_bbr
    if [ $? != 0 ]; then 
        return 1
    fi 

    create_cert
    if [ $? != 0 ]; then 
        return 1
    fi 

    install_docker
    if [ $? != 0 ]; then 
        return 1
    fi 

#    install_yeager
#    if [ $? != 0 ]; then
#        return 1
#    fi
}

main
