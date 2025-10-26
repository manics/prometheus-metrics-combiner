# Prometheus Metrics Combiner
[![Build](https://github.com/manics/prometheus-metrics-combiner/actions/workflows/build.yml/badge.svg)](https://github.com/manics/prometheus-metrics-combiner/actions/workflows/build.yml)

A simple HTTP server that fetches Prometheus metrics from multiple upstream services, combines them, and exposes them on a single `/metrics` endpoint.

For example, if you have a Kubernetes pod with multiple containers with separate metrics endpoints you can use this to expose a single endpoint for scraping.

## Building

This requires Go 1.25

```bash
go build
```

## Usage

The server is configured via command-line flags.

### Command-Line Flags

- `-port <number>`: The port for the HTTP server to listen on (default `8080`)
- `-url <url>`: An upstream URL to fetch metrics from, can be specified multiple times
- `-prefix <string>`: Optional filter, only lines starting with this prefix will be included in the output, can be specified multiple times
