PROTO_DIR  := proto
PB_DIR     := statestream/pb
PB_PKG     := github.com/douglascamata/busylib-go/$(PB_DIR)
PROTO_REPO := https://github.com/flipperdevices/bsb-protobuf.git
PROTOS     := $(shell cd $(PROTO_DIR) && find . -name '*.proto' | sed 's|^\./||' | sort)
GOLANGCI   := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
CHANGIE    := go run github.com/miniscruff/changie@v1.26.0

.PHONY: build test smoke-test lint generate-proto update-protos \
	change release-notes release-tag release-push release require-version

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

## change: write a changelog fragment for an unreleased change (interactive)
change:
	$(CHANGIE) new

## release-notes: batch unreleased fragments into .changes/$(VERSION).md and rebuild CHANGELOG.md
release-notes: require-version
	$(CHANGIE) batch $(VERSION)
	$(CHANGIE) merge

## release-tag: commit the release notes and create the annotated tag $(VERSION)
release-tag: require-version
	@test -f .changes/$(VERSION).md || { echo ".changes/$(VERSION).md not found, run: make release-notes VERSION=$(VERSION)"; exit 1; }
	git add CHANGELOG.md .changes
	git commit -m "Release $(VERSION)" -- CHANGELOG.md .changes
	git tag -a $(VERSION) -m "$(VERSION)"

## release-push: push the release commit and the tag $(VERSION)
release-push: require-version
	git push --atomic origin HEAD $(VERSION)

## release: release-notes, release-tag and release-push in one go
release: require-version
	$(MAKE) release-notes VERSION=$(VERSION)
	$(MAKE) release-tag VERSION=$(VERSION)
	$(MAKE) release-push VERSION=$(VERSION)

require-version:
	@case "$(VERSION)" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo "usage: make $(MAKECMDGOALS) VERSION=vX.Y.Z"; exit 1;; esac
