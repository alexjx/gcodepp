.PHONY: all build compile clean

LDFLAGS = -extldflags \
          -static

all: build

clean:
	rm -rf bin

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/gcodepp -ldflags "$(LDFLAGS)" .

release:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/gcodepp-win.exe -ldflags "$(LDFLAGS)" .