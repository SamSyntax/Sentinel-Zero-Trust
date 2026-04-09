BUILD_DIR=bin
BIN_NAME=sentinel-init
DOCKERFILE=Dockerfile
IMAGE_NAME=kind.local/sentinel-init:latest

.PHONY: all build clean

all: clean build

build: 
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BIN_NAME) ./cmd

clean: 
	@test ! -d $(BUILD_DIR) || rm -rf $(BUILD_DIR)/*
	@rm -rf $(BUILD_DIR)/*

build-docker: all
	docker build -f $(DOCKERFILE) -t $(IMAGE_NAME) .
