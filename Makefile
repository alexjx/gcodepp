.PHONY: all build compile clean


BUILDTIME ?= $(shell date +%Y-%m-%d_%I:%M:%S)
GITCOMMIT ?= $(shell git rev-parse HEAD)
BUILDNUMER ?= $(shell git rev-list --count HEAD)

LDFLAGS = -extldflags \
          -static \
          -X "main.BuiltAt=$(BUILDTIME)" \
          -X "main.GitHash=$(GITCOMMIT)" \
          -X "main.BuildNumber=$(BUILDNUMER)"

all: build

clean:
	rm -rf bin

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/gcodepp -ldflags "$(LDFLAGS)" .

release:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/gcodepp-win.exe -ldflags "$(LDFLAGS)" .