FROM golang:alpine

WORKDIR /go/src/github.com/honeydipper/honeydipper

RUN apk add --no-cache git gcc libc-dev
COPY . .
RUN go get -u github.com/golang/dep/cmd/dep && dep ensure
RUN go install ./...

ENTRYPOINT ["honeydipper"]
