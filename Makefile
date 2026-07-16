.DEFAULT_GOAL := test

# gofmt does not read go.mod's `ignore ./agent` directive, so the formatting
# check excludes the scaffolding tree (and dot-dirs) by path.
GOFILES := $(shell find . -name '*.go' -not -path './agent/*' -not -path './.*/*')

# test runs the unit tests with the race detector and coverage.
.PHONY: test
test:
	go test -race -cover ./...

# cover-profile writes coverage.out (also used by CI for the Codecov upload).
.PHONY: cover-profile
cover-profile:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...

# cover writes a coverage profile and opens the HTML report.
.PHONY: cover
cover: cover-profile
	go tool cover -html=coverage.out

# check runs formatting verification and static analysis.
.PHONY: check
check: fmt vet

# fmt fails if any file is not gofmt-clean.
.PHONY: fmt
fmt:
	@unformatted=$$(gofmt -l $(GOFILES)); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi

# vet runs go vet across the module.
.PHONY: vet
vet:
	go vet ./...

# tidy ensures go.mod / go.sum are in sync with the source.
.PHONY: tidy
tidy:
	go mod tidy

# clean removes generated coverage artifacts.
.PHONY: clean
clean:
	rm -f coverage.out
