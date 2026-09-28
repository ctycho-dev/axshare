BIN     := axshare
ADDR    ?= :8070
DB      ?= axshare.db

.PHONY: run race build test vet fmt check health clean

run:            ## start the server
	go run ./cmd/$(BIN) -addr $(ADDR) -db $(DB)

race:           ## start the server under the race detector
	go run -race ./cmd/$(BIN) -addr $(ADDR) -db $(DB)

build:          ## build a static binary into ./bin
	CGO_ENABLED=0 go build -o bin/$(BIN) ./cmd/$(BIN)

test:           ## run all tests with the race detector
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

check: fmt vet test   ## run before every commit

health:         ## poke the running server
	@curl -s localhost$(ADDR)/healthz; echo

clean:
	rm -rf bin $(DB) $(DB)-wal $(DB)-shm