FROM golang:latest

WORKDIR /go
ENV GOPATH=/go
ENV GOBIN=/go/bin
RUN mkdir -p /go/bin
COPY ./src ./src
COPY main_raft.go ./
COPY main_client.go ./

# Notice that hostfile needs to be in the directory where binary gets invoked
COPY ./hostfile.json /go/hostfile.json
COPY ./datafile.0.txt /go/datafile.0.txt
COPY ./datafile.1.txt /go/datafile.1.txt
RUN go build -o /bin/raft main_raft.go
RUN go build -o /bin/client main_client.go

ENV PATH="/go/bin:${PATH}"
