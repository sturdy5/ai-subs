#!/bin/bash
# This will update all the go dependencies to their latest versions and tidy up the go.mod file.
# It will also update the go.sum file to reflect the changes.

go get -u ./...
go mod tidy
