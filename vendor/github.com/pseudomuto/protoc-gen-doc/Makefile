.PHONY: bench setup test build dist docker examples release

EXAMPLE_DIR=$(PWD)/examples
DOCS_DIR=$(EXAMPLE_DIR)/doc
PROTOS_DIR=$(EXAMPLE_DIR)/proto

EXAMPLE_CMD=protoc --plugin=protoc-gen-doc -Ivendor -Itmp/googleapis -Iexamples/proto --doc_out=examples/doc
DOCKER_CMD=docker run --rm -v $(DOCS_DIR):/out:rw -v $(PROTOS_DIR):/protos:ro -v $(EXAMPLE_DIR)/templates:/templates:ro -v $(PWD)/vendor/github.com/mwitkow:/usr/local/include/github.com/mwitkow:ro -v $(PWD)/vendor/github.com/lyft:/usr/local/include/github.com/lyft:ro -v $(PWD)/tmp/googleapis/google/api:/usr/local/include/google/api:ro pseudomuto/protoc-gen-doc

VERSION = $(shell cat version.go | sed -n 's/.*const VERSION = "\(.*\)"/\1/p')

setup:
	$(info Synching dev tools and dependencies...)
	@if test -z $(which retool); then go get github.com/twitchtv/retool; fi
	@retool sync
	@retool do dep ensure

resources.go: resources/*.tmpl resources/*.json
	$(info Generating resources...)
	@go run resources/main.go -in resources -out resources.go -pkg gendoc

fixtures/fileset.pb: fixtures/*.proto fixtures/generate.go
	$(info Generating fixtures...)
	@cd fixtures && go generate

tmp/googleapis:
	rm -rf tmp/googleapis
	git clone --depth 1 https://github.com/googleapis/googleapis tmp/googleapis

test: fixtures/fileset.pb resources.go
	@go test -cover -race ./ ./cmd/...

bench:
	@go test -bench=.

build: setup resources.go
	@go build ./cmd/...

dist:
	@script/dist.sh

docker:
	@script/push_to_docker.sh

docker_test: build tmp/googleapis docker
	@rm -f examples/doc/*
	@$(DOCKER_CMD) --doc_opt=docbook,example.docbook:Ignore*
	@$(DOCKER_CMD) --doc_opt=html,example.html:Ignore*
	@$(DOCKER_CMD) --doc_opt=json,example.json:Ignore*
	@$(DOCKER_CMD) --doc_opt=markdown,example.md:Ignore*
	@$(DOCKER_CMD) --doc_opt=/templates/asciidoc.tmpl,example.txt:Ignore*

examples: build tmp/googleapis examples/proto/*.proto examples/templates/*.tmpl
	$(info Making examples...)
	@rm -f examples/doc/*
	@$(EXAMPLE_CMD) --doc_opt=docbook,example.docbook:Ignore* examples/proto/*.proto
	@$(EXAMPLE_CMD) --doc_opt=html,example.html:Ignore* examples/proto/*.proto
	@$(EXAMPLE_CMD) --doc_opt=json,example.json:Ignore* examples/proto/*.proto
	@$(EXAMPLE_CMD) --doc_opt=markdown,example.md:Ignore* examples/proto/*.proto
	@$(EXAMPLE_CMD) --doc_opt=examples/templates/asciidoc.tmpl,example.txt:Ignore* examples/proto/*.proto

release:
	@echo Releasing v${VERSION}...
	git add CHANGELOG.md version.go
	git commit -m "Bump version to v${VERSION}"
	git tag -m "Version ${VERSION}" "v${VERSION}"
	git push && git push --tags
