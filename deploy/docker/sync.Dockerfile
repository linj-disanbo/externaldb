FROM ubuntu:22.04

LABEL author="linj" 
LABEL module="externaldb/sync"

RUN  apt-get update -y && apt-get install -y curl

WORKDIR /root
COPY sync externaldb.toml ci-start-sync.sh ./
