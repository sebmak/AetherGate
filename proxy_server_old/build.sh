#!/bin/bash
docker buildx build --platform=linux/amd64 --push -t 192.168.1.104:5000/proxy_server:${1:-latest} . --output=type=registry,registry.insecure=true
echo ${1:-latest} > VERSION