FROM --platform=$BUILDPLATFORM golang:1.24.4-alpine AS base

RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid 65532 \
    small-user

RUN apk add --no-cache tzdata
## Needed if downstream users want to export metrics to f.ex. cloudwatch
RUN apk update 
RUN apk add curl tar 

WORKDIR $GOPATH/src/smallest-golang/app/

COPY . .

RUN go mod download
RUN go mod verify

ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /ad-normalizer ./cmd/ad-normalizer

USER small-user:small-user
ENV TZ=GMT

CMD ["/ad-normalizer"]