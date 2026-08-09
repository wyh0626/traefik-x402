.PHONY: fmt vet test test-race e2e

fmt:
	gofmt -w .

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

e2e:
	./e2e/run.sh
