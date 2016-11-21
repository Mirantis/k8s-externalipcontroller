FROM alpine:3.4
MAINTAINER Dmitry Shulyak <yashulyak@gmail.com>
LABEL Name="k8s-externalipcontroller" Version="0.1"

COPY _output/ipmanager /usr/sbin/

CMD ["sh", "-c", "/usr/sbin/ipmanager n --alsologtostderr=true --v=4 --iface=${HOST_INTERFACE}"]
