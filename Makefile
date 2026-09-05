PROTO_DIR  := proto
PB_DIR     := statestream/pb
PB_PKG     := github.com/douglascamata/busylib-go/$(PB_DIR)
PROTO_REPO := https://github.com/flipperdevices/bsb-protobuf.git
PROTOS     := $(shell cd $(PROTO_DIR) && find . -name '*.proto' | sed 's|^\./||' | sort)
GOLANGCI   := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2

.PHONY: build test smoke-test lint generate-proto update-protos

## build: compile every package (this is a library, there is no binary)
build:
	go build ./...

## test: tests with the race detector
test:
	go test -race ./...

## smoke-test: boot busybar-emulator and run the smoke tests against it
smoke-test:
	scripts/smoke.sh

## lint: gofmt, go vet and golangci-lint
lint:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	go vet -tags smoke ./...
	$(GOLANGCI) run --build-tags smoke ./...

## generate-proto: regenerate $(PB_DIR) from $(PROTO_DIR) (needs protoc and protoc-gen-go)
generate-proto:
	protoc -I $(PROTO_DIR) --go_out=. --go_opt=module=$(shell go list -m) \
		$(foreach f,$(PROTOS),--go_opt=M$(f)=$(PB_PKG)) $(PROTOS)

## update-protos: fetch the latest upstream schemas, then regenerate
update-protos:
	rm -rf $(PROTO_DIR)/tmp
	git clone --quiet --depth 1 $(PROTO_REPO) $(PROTO_DIR)/tmp
	find $(PROTO_DIR) -name '*.proto' -not -path '$(PROTO_DIR)/tmp/*' -delete
	cd $(PROTO_DIR)/tmp && find . -name '*.proto' | while read -r f; do mkdir -p "../$$(dirname "$$f")"; cp "$$f" "../$$f"; done
	cp $(PROTO_DIR)/tmp/LICENSE.md $(PROTO_DIR)/LICENSE.md
	git -C $(PROTO_DIR)/tmp rev-parse HEAD > $(PROTO_DIR)/UPSTREAM_COMMIT
	rm -rf $(PROTO_DIR)/tmp
	$(MAKE) generate-proto
