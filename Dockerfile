FROM golang:latest

WORKDIR /go
ENV GOPATH=/go
ENV GOBIN=/go/bin
RUN mkdir -p /go/bin
COPY ./src ./src
COPY main_raft.go ./
COPY main_client.go ./

# Notice that hostfile needs to be in the directory where binary gets invoked
COPY ./hostfile.txt /go/hostfile.txt
COPY ./datafile.txt /go/datafile.txt
RUN go build -o /bin/raft main_raft.go
RUN go build -o /bin/client main_client.go

ENV PATH="/go/bin:${PATH}"

ENTRYPOINT ["raft"]
