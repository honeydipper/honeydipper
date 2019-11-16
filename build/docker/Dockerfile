FROM golang:1.13.4-alpine AS build

ARG GOINSTALLOPTS=
ARG GOGC=

ENV GOGC=$GOGC

WORKDIR /go/src/github.com/honeydipper/honeydipper

RUN apk add --no-cache git gcc libc-dev
COPY ./ ./
RUN go get -u github.com/golang/dep/cmd/dep && dep ensure
RUN go install $GOINSTALLOPTS ./...

FROM alpine:3.10

LABEL description="Honeydipper - an event-driven orchestration framework" \
      org.label-schema.vcs-url=https://github.com/honeydipper/honeydipper \
      org.label-schema.schema-version="1.0"

RUN apk add --no-cache ca-certificates git

WORKDIR /opt/honeydipper/drivers/builtin
COPY --from=build /go/bin/* ./

ENTRYPOINT ["./honeydipper"]
