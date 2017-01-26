FROM alpine
RUN apk add --no-cache --update ca-certificates
ADD aliyun-controller /aliyun-controller
ADD flexv /flexv
