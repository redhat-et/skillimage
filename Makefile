.PHONY: build test lint fmt clean image

BINARY := skillctl
BINDIR := bin

build:
	go build -o $(BINDIR)/$(BINARY) ./cmd/skillctl

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	gofumpt -l -w .

image:
	podman -c rhel build -f Dockerfile.local -t ghcr.io/redhat-et/skillctl:latest .

clean:
	rm -rf $(BINDIR)
