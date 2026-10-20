.PHONY: build test validate schemas

build:
	go build -trimpath -o bin/go-clean-deadcode ./cmd/go-clean-deadcode

test:
	go test ./...

validate:
	go test ./...
	go vet ./...
	python3 scripts/check_skills.py

schemas:
	go run ./scripts/schema-gen
