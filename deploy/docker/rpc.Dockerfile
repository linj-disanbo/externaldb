FROM ubuntu:22.04

LABEL author="linj" 
LABEL module="externaldb/rpc"

WORKDIR /root
COPY rpc externaldb.toml ./
