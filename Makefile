test:
	go test -v ./...

build:
	go build -o file-mover-daemon cmd/main.go
