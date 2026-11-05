BIN      := cream
VERSION  ?= v$(shell cat VERSION)-dev
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(shell git rev-parse --short HEAD 2>/dev/null) -X main.date=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: build test cover lint vuln run snapshot clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BIN) .

test:
	go test -count=1 ./...

cover:
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -n 1

lint:
	golangci-lint run ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# throwaway home real config is untouched
run: build
	CREAM_HOME=$$(mktemp -d) CREAM_NO_UPDATE_CHECK=1 ./bin/$(BIN)

snapshot:
	goreleaser release --snapshot --clean --skip=sign,sbom,docker,publish

clean:
	rm -rf bin dist coverage.out
