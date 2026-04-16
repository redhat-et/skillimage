.PHONY: build test lint fmt clean

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

clean:
	rm -rf $(BINDIR)
