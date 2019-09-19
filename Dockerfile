FROM golang:1.13.0 as builder
WORKDIR /go/src/github.com/trivialdns
COPY ./ /go/src/github.com/trivialdns/
RUN go get -d -v
RUN CGO_ENABLED=0 GOOS=linux go build

FROM alpine:3.10
COPY --from=builder /go/src/github.com/trivialdns/trivialdns /usr/bin/trivialdns
RUN mkdir -p /etc/trivialdns && \
    echo "8.8.8.8" > /etc/trivialdns/nameservers && \
    echo "8.8.4.4" >> /etc/trivialdns/nameservers
EXPOSE 8053
ENTRYPOINT ["trivialdns"]
