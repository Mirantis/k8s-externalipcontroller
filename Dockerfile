FROM ubuntu:16.04
MAINTAINER Dmitry Shulyak <yashulyak@gmail.com>
LABEL Name="externalipcontroller" Version="0.1"

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

RUN go build cmd/ipcontroller.go

CMD ["./ipcontroller", "--alsologtostderr=true", "-v=4", "-iface=echo ${HOST_INTERFACE}"]