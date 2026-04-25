.PHONY: build test lint fmt clean image deploy

BINARY := skillctl
BINDIR := bin
IMAGE  := ghcr.io/redhat-et/skillctl:latest

build:
	go build -o $(BINDIR)/$(BINARY) ./cmd/skillctl

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	gofumpt -l -w .

image:
	podman -c rhel build -f Dockerfile.local -t $(IMAGE) .

deploy: image
	podman -c rhel push $(IMAGE)
	oc rollout restart deploy/skillctl-catalog
	oc rollout status deploy/skillctl-catalog --timeout=60s
	@echo "---"
	@echo "Route: https://$$(oc get route skillctl-catalog -o jsonpath='{.spec.host}')/api/v1/skills"
	@echo "Run 'oc logs -f deploy/skillctl-catalog' to tail logs"

clean:
	rm -rf $(BINDIR)
