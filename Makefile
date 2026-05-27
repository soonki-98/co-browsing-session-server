.PHONY: openapi openapi-check dev

openapi:
	go run ./cmd/gen-openapi

openapi-check: openapi
	@git diff --exit-code docs/openapi.yaml || \
	  (echo "docs/openapi.yaml is stale. Run 'make openapi' and commit." && exit 1)

dev:
	air
