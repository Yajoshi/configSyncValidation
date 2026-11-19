.PHONY: build test vet tidy

build: tidy
	go build -o configsync-validator .

tidy:
	go mod tidy

vet:
	go vet ./...

test:
	go test -race ./...
