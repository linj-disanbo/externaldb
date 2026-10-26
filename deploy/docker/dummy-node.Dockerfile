FROM ubuntu:22.04

LABEL author="linj" 
LABEL module="externaldb/dummy-node"

WORKDIR /root
COPY ./dummy_node ./
