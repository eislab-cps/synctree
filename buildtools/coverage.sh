#!/usr/bin/env bash

set -e
echo "" > coverage.txt

go test -race -coverprofile=profile.out -covermode=atomic ./internal/crypto 
if [ -f profile.out ]; then
  cat profile.out >> coverage.txt
  rm profile.out
fi
go test -race -coverprofile=profile.out -covermode=atomic ./pkg/crdt
if [ -f profile.out ]; then
  cat profile.out >> coverage.txt
  rm profile.out
fi
