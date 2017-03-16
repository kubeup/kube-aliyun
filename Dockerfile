FROM debian:jessie
RUN apt update && apt install -y ca-certificates
ADD aliyun-controller /aliyun-controller
ADD flexv /flexv
