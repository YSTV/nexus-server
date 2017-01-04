#!/usr/bin/env bash

go get github.com/mitchellh/gox
gox -ldflags="-X main.VERSION=`git describe --tags`" -output "dist/nexus-server_{{.OS}}_{{.Arch}}" -osarch="linux/amd64 linux/arm darwin/amd64 freebsd/amd64 windows/amd64"

