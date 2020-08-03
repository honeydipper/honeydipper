FROM golang:1.14.6-alpine AS build

ARG GOINSTALLOPTS
ARG GOGC

ENV GOGC=$GOGC

# Enable module mode (see https://github.com/golang/go/wiki/Modules)
ENV GO111MODULE=auto

WORKDIR /build

RUN apk add --no-cache git gcc libc-dev
COPY ./ ./
RUN go install -v ./drivers/cmd/gcloud-pubsub && \
    go install -v ./drivers/cmd/redisqueue && \
    go install -v ./drivers/cmd/redispubsub && \
    go install -v ./drivers/cmd/gcloud-storage && \
    go install -v ./drivers/cmd/kubernetes && \
    go install -v ./cmd/... && \
    go install -v ./drivers/...

FROM alpine:3.12

LABEL description="Honeydipper - an event-driven orchestration framework" \
      org.label-schema.vcs-url=https://github.com/honeydipper/honeydipper \
      org.label-schema.schema-version="1.0"

RUN apk add --no-cache ca-certificates git

WORKDIR /opt/honeydipper/drivers/builtin
COPY --from=build /go/bin/* ./

ENTRYPOINT ["./honeydipper"]
