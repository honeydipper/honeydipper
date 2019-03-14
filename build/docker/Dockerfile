FROM golang:1.11-alpine AS build

WORKDIR /go/src/github.com/honeydipper/honeydipper

RUN apk add --no-cache git gcc libc-dev
COPY ./ ./
RUN go get -u github.com/golang/dep/cmd/dep && dep ensure
RUN go install ./... && rm /go/bin/dep

FROM alpine:3.9

LABEL description="Honeydipper - an event-driven orchestration framework" \
      org.label-schema.vcs-url=https://github.com/honeydipper/honeydipper \
      org.label-schema.schema-version="1.0"

RUN apk add --no-cache ca-certificates git

WORKDIR /opt/honeydipper/drivers/builtin
COPY --from=build /go/bin/* ./

ENTRYPOINT ["./honeydipper"]
