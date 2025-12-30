#!/usr/bin/env bash

set -e -x

GO_OPT=paths=source_relative
GRPC_OPT=paths=source_relative

protoc -I . --go_out=. --go_opt=$GO_OPT \
  ./afterpiece/parser/meta/v1/meta.proto

protoc -I . --go_out=. --go_opt=$GO_OPT \
  ./afterpiece/parser/schema/v1/schema.proto

protoc -I . --go_out=. --go_opt=$GO_OPT \
  ./afterpiece/engine/trace/trace.proto

protoc -I . --go_out=. --go_opt=$GO_OPT \
  ./afterpiece/engine/trace2/trace2.proto

protoc -I . --go_out=. --go_opt=$GO_OPT --go-grpc_out=. --go-grpc_opt=$GRPC_OPT \
  ./afterpiece/daemon/daemon.proto

protoc -I . --go_out=. --go_opt=$GO_OPT \
./afterpiece/runtime/v1/infra.proto

protoc -I . --go_out=. --go_opt=$GO_OPT \
./afterpiece/runtime/v1/runtime.proto

protoc -I . --go_out=. --go_opt=$GO_OPT \
./afterpiece/runtime/v1/secretdata.proto

protoc -I . --go_out=. --go_opt=$GO_OPT \
./afterpiece/runtime/v1/secretdata.proto
