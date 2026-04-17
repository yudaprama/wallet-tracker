APP=wallet-tracker
MAIN=./cmd/wallet-tracker
CONTRACT?=0x17205fab260a7a6383a81452cE6315A39370Db97
LIMIT?=20
FORMAT?=table
TOP?=20
ADDRESSES?=
ADDRESSES_FILE?=data/rave-addresses.txt
INTERVAL?=5m
ITERATIONS?=0

.PHONY: help run run-help token-holders token-holders-json token-balances token-balances-json token-snapshot token-diff token-watch build

help:
	@printf "Targets:\n"
	@printf "  make run ARGS='tracker token-holders --contract <address> --limit 20'\n"
	@printf "  make token-holders CONTRACT=<address> LIMIT=20\n"
	@printf "  make token-holders-json CONTRACT=<address> LIMIT=20\n"
	@printf "  make token-balances CONTRACT=<address> ADDRESSES_FILE=data/rave-addresses.txt TOP=20\n"
	@printf "  make token-snapshot CONTRACT=<address> ADDRESSES_FILE=data/rave-addresses.txt\n"
	@printf "  make token-diff CONTRACT=<address>\n"
	@printf "  make token-watch CONTRACT=<address> INTERVAL=5m ITERATIONS=0\n"
	@printf "  make build\n"

run:
	go run $(MAIN) $(ARGS)

run-help:
	go run $(MAIN) --help

token-holders:
	go run $(MAIN) tracker token-holders --contract $(CONTRACT) --limit $(LIMIT) --format $(FORMAT)

token-holders-json:
	go run $(MAIN) tracker token-holders --contract $(CONTRACT) --limit $(LIMIT) --format json

token-balances:
	go run $(MAIN) tracker token-balances --contract $(CONTRACT) --addresses "$(ADDRESSES)" --addresses-file $(ADDRESSES_FILE) --top $(TOP) --format $(FORMAT)

token-balances-json:
	go run $(MAIN) tracker token-balances --contract $(CONTRACT) --addresses "$(ADDRESSES)" --addresses-file $(ADDRESSES_FILE) --top $(TOP) --format json

token-snapshot:
	go run $(MAIN) tracker token-snapshot --contract $(CONTRACT) --addresses "$(ADDRESSES)" --addresses-file $(ADDRESSES_FILE)

token-diff:
	go run $(MAIN) tracker token-diff --contract $(CONTRACT)

token-watch:
	./wallet-tracker tracker token-watch --contract $(CONTRACT) --addresses "$(ADDRESSES)" --addresses-file $(ADDRESSES_FILE) --interval $(INTERVAL) --iterations $(ITERATIONS)

build:
	go build -o $(APP) $(MAIN)
