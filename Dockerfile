FROM --platform=$BUILDPLATFORM golang:1.24.4 AS base

RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid 65532 \
    small-user

WORKDIR $GOPATH/src/smallest-golang/app/

COPY . .

RUN go mod download
RUN go mod verify

ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOD=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /ad-normalizer ./cmd/ad-normalizer

FROM --platform=$TARGETPLATFORM scratch

COPY --from=base /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=base /etc/group /etc/group
COPY --from=base /ad-normalizer .

USER small-user:small-user
ENV TZ=GMT

CMD ["./ad-normalizer"]