FROM golang:latest

WORKDIR /go
ENV GOPATH=/go
ENV GOBIN=/go/bin
RUN mkdir -p /go/bin
COPY ./src ./src
COPY main.go ./

# Notice that hostfile needs to be in the directory where binary gets invoked
COPY ./hostfile.txt /go/hostfile.txt
#WORKDIR /go/src
RUN go build -o /bin/raft main.go

#WORKDIR /go
ENV PATH="/go/bin:${PATH}"

ENTRYPOINT ["raft"]
