FROM golang:1.25.10-alpine AS build

ARG GOINSTALLOPTS=""
ARG GOGC=""

ARG HD_VERSION=""

ENV GOGC=$GOGC

# Enable module mode (see https://github.com/golang/go/wiki/Modules)
ENV GO111MODULE=auto

WORKDIR /build

RUN apk add --no-cache git gcc libc-dev
COPY ./ ./
RUN go install -v ./drivers/cmd/gcloud-pubsub
RUN go install -v ./drivers/cmd/gcloud-spanner
RUN go install -v ./drivers/cmd/gcloud-storage
RUN go install -v ./drivers/cmd/kubernetes
RUN go install -v ./drivers/cmd/redisqueue
RUN go install -v ./drivers/cmd/redispubsub
RUN if [[ -z "$HD_VERSION" ]]; then HD_VERSION="$(git describe --tags)"; fi && \
    go install -v -ldflags='-X main._honeydipperVersion='$HD_VERSION ./cmd/...
RUN go install -v ./drivers/...

FROM alpine:3.21

LABEL description="Honeydipper - an event-driven orchestration framework" \
      org.label-schema.vcs-url=https://github.com/honeydipper/honeydipper \
      org.label-schema.schema-version="1.0"

RUN apk add --no-cache ca-certificates git tzdata

WORKDIR /opt/honeydipper/drivers/builtin
COPY --from=build /go/bin/* ./
COPY ./build/docker/docker-entrypoint.sh ./

ENTRYPOINT ["./docker-entrypoint.sh"]
EXPOSE 8080
EXPOSE 9000
