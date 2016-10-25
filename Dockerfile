FROM ubuntu:16.04
MAINTAINER Dmitry Shulyak <yashulyak@gmail.com>
LABEL Name="k8s-externalipcontroller" Version="0.1"

RUN apt-get update \
	&& apt-get install -y software-properties-common \
	&& apt-get clean
RUN add-apt-repository ppa:ubuntu-lxc/lxd-stable
RUN DEBIAN_FRONTEND=noninteractive apt-get install -y golang \
	&& apt-get clean

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

WORKDIR $GOPATH

RUN mkdir -p /go/src/github.com/Mirantis/k8s-externalipcontroller
COPY . /go/src/github.com/Mirantis/k8s-externalipcontroller

WORKDIR /go/src/github.com/Mirantis/k8s-externalipcontroller

RUN go build -o /usr/local/bin/ipcontroller cmd/ipcontroller.go

CMD ["sh", "-c", "/usr/local/bin/ipcontroller --alsologtostderr=true -v=4 -iface=${HOST_INTERFACE}"]
