IMAGE_NAME := "ghcr.io/jkahrs/cert-manager-webhook-hostingde"
IMAGE_TAG := "latest"

OUT := $(shell pwd)/_out
TEST_BIN := $(shell pwd)/kubebuilder/bin

$(shell mkdir -p "$(OUT)")

verify:
	TEST_ASSET_ETCD=$(TEST_BIN)/etcd \
	TEST_ASSET_KUBE_APISERVER=$(TEST_BIN)/kube-apiserver \
	TEST_ASSET_KUBECTL=$(TEST_BIN)/kubectl \
	go test -v .

build:
	docker build -t "$(IMAGE_NAME):$(IMAGE_TAG)" .

clean:
	go clean
	go clean -testcache
