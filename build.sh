#!/bin/bash

go build -o ai-subs main.go
GOOS=windows GOARCH=amd64 go build -o ai-subs.exe main.go
