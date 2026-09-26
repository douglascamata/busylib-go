PROTO_DIR  := proto
PB_DIR     := statestream/pb
PB_PKG     := github.com/douglascamata/busylib-go/$(PB_DIR)
PROTO_REPO := https://github.com/flipperdevices/bsb-protobuf.git
PROTOS     := $(shell cd $(PROTO_DIR) && find . -name '*.proto' | sed 's|^\./||' | sort)
GOLANGCI   := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
CHANGIE    := go run github.com/miniscruff/changie@v1.26.0
ZIZMOR     := zizmor

.PHONY: build test smoke-test lint lint-actions generate-proto update-protos change changelog check-changes update-changelog prepare-release

## build: compile every package (this is a library, there is no binary)
build:
	go build ./...

## test: tests with the race detector
test:
	go test -race ./...

## smoke-test: boot busybar-emulator and run the smoke tests against it
smoke-test:
	scripts/smoke.sh

## lint: gofmt, go vet, golangci-lint and lint-actions
lint: lint-actions
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	go vet -tags smoke ./...
	$(GOLANGCI) run --build-tags smoke ./...

## lint-actions: audit the GitHub Actions workflows with zizmor (CI runs this through lint)
# Online audits need a GitHub token; without one zizmor runs offline.
lint-actions:
	@token="$${GH_TOKEN:-$$(gh auth token 2>/dev/null || true)}"; \
	env $${token:+GH_TOKEN="$$token"} $(ZIZMOR) --persona=pedantic .github/workflows

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

## change: add a changelog fragment (prompts; ARGS='--kind Fixed --body "..."' skips them)
change:
	$(CHANGIE) new $(ARGS)

## changelog: preview the next version and its release notes
changelog:
	$(CHANGIE) batch auto --dry-run

## check-changes: validate the unreleased changelog fragments
check-changes:
	$(CHANGIE) batch patch --dry-run --allow-no-changes >/dev/null

## update-changelog: rebuild CHANGELOG.md from the released versions in .changes/
update-changelog:
	$(CHANGIE) merge

## prepare-release: commit the next release locally; never tags or pushes (VERSION=vX.Y.Z overrides the bump)
prepare-release:
	CHANGIE="$(CHANGIE)" scripts/prepare-release.sh $(VERSION)
