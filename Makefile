BIN     := axshare
ADDR    ?= :8070
DB      ?= axshare.db

.PHONY: run race build test vet fmt check health clean web dev docker up down

run:            ## start the server
	go run ./cmd/$(BIN) -addr $(ADDR) -db $(DB)

race:           ## start the server under the race detector
	go run -race ./cmd/$(BIN) -addr $(ADDR) -db $(DB)

build: web      ## build frontend, then a static binary into ./bin
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

docker:         ## build the image
	docker build -t axshare:latest .

up:             ## run via compose in the background
	docker compose up -d --build

down:
	docker compose down

web:            ## build the frontend into internal/server/dist
	cd web && npm ci --silent && npm run build

dev:            ## vite dev server with hot reload; run `make run` alongside
	cd web && npm run dev