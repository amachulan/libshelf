.PHONY: build build-linux test tidy

build:
	go build -o bin/libshelf ./cmd/libshelf

build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/libshelf-linux-amd64 ./cmd/libshelf

test:
	go test ./...

tidy:
	go mod tidy
