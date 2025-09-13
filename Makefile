.PHONY: test run build clean

test:
	go test -v ./...

run:
	go run cmd/server/main.go

build:
	go build -o bin/k8s-node-proxy cmd/server/main.go

clean:
	rm -rf bin/
