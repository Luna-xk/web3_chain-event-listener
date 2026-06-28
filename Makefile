.PHONY: build run demo test tidy fmt clean

BIN := bin/listener

build:
	go build -o $(BIN) ./cmd/listener

# 离线演示:使用 mock 模式,无需任何外部依赖。
demo: build
	@test -f config.yaml || cp config.example.yaml config.yaml
	./$(BIN) -config config.yaml

run: build
	./$(BIN) -config config.yaml

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

clean:
	rm -rf bin
