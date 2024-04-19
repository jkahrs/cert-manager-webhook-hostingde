IMAGE_NAME := "ghcr.io/jkahrs/cert-manager-webhook-hostingde"
IMAGE_TAG := "latest"

OUT := $(shell pwd)/_out

$(shell mkdir -p "$(OUT)")

verify:
	go test -v .

build:
	docker build -t "$(IMAGE_NAME):$(IMAGE_TAG)" .

clean:
	go clean
	go clean -testcache
