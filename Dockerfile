FROM debian:jessie
MAINTAINER Dmitry Shulyak <yashulyak@gmail.com>
LABEL Name="k8s-externalipcontroller" Version="0.1"

COPY _output/ipmanager /usr/local/bin/

CMD ["sh", "-c", "/usr/local/bin/ipmanager n --alsologtostderr=true --v=4 --iface=${HOST_INTERFACE}"]
