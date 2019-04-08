#!/usr/bin/env sh

protoc shortsvc.proto --go_out=plugins=grpc:.
