build:
	go build -o fastship ./cmd/fastship

run: build
	./fastship

test:
	go test ./...

clean:
	rm -f fastship