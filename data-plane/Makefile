.PHONY: all build clean run

BIN_NAME=plane
BUILD_DIR=bin

all: clean build run

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BIN_NAME) ./cmd

run: build
	./$(BUILD_DIR)/$(BIN_NAME)

clean:
	rm -rf $(BUILD_DIR)


