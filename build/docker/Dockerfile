FROM golang:1.14.4-alpine AS build

ARG GOINSTALLOPTS
ARG GOGC

ENV GOGC=$GOGC

# Enable module mode (see https://github.com/golang/go/wiki/Modules)
ENV GO111MODULE=auto

WORKDIR /build

RUN apk add --no-cache git gcc libc-dev
COPY ./ ./
RUN go install -v ./...

FROM alpine:3.12

LABEL description="Honeydipper - an event-driven orchestration framework" \
      org.label-schema.vcs-url=https://github.com/honeydipper/honeydipper \
      org.label-schema.schema-version="1.0"

RUN apk add --no-cache ca-certificates git

WORKDIR /opt/honeydipper/drivers/builtin
COPY --from=build /go/bin/* ./

ENTRYPOINT ["./honeydipper"]
