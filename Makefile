# only for developer testing (the production image would be build by github action)
local_image:
	# 禁用CGO，避免程序在alpine镜像运行的时候找不到动态链接库
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o release/yeager .
	docker build . --file Dockerfile --tag yeager
	rm release/yeager
