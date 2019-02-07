FROM golang:alpine

WORKDIR /go/src/github.com/honeyscience/honeydipper

RUN apk add git gcc libc-dev
COPY . .
RUN go get -u github.com/golang/dep/cmd/dep && dep ensure
RUN go install ./...

ENTRYPOINT ["honeydipper"]
