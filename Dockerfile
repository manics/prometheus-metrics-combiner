FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

WORKDIR /app

# There are no dependencies, so no go.sum
COPY go.mod *.go ./

ARG TARGETARCH
# Creating a static binary. -ldflags="-w -s" reduces the binary size.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH \
  go build -a -ldflags="-w -s" -o /app/prometheus-metrics-combiner .

######################################################################

FROM alpine:latest
COPY --from=builder /app/prometheus-metrics-combiner /prometheus-metrics-combiner
EXPOSE 8080
ENTRYPOINT ["/prometheus-metrics-combiner"]
