#!/bin/bash

cp ../auth/proto_files/* ./proto_files/

protoc --go_out=./pkg/proto --go-grpc_out=./pkg/proto ./proto_files/*.proto