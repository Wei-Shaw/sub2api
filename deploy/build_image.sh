#!/bin/bash
# 本地构建镜像的快速脚本，避免在命令行反复输入构建参数。

docker build -t sub2api:latest \
    --build-arg GOPROXY=https://goproxy.cn,direct \
    --build-arg GOSUMDB=sum.golang.google.cn \
    -f Dockerfile \
    .
