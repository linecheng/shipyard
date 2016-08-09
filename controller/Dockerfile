FROM ubuntu:14.04
#RUN apk add --update git ca-certificates && \
#    rm -rf /var/cache/apk/*
ADD static /static
ADD controller /bin/controller
ENV TZ Asia/Shanghai
EXPOSE 8080
ENTRYPOINT ["/bin/controller"]
