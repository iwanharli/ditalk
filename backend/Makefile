GOOSE_DRIVER   ?= postgres
GOOSE_DBSTRING ?= postgres://localhost:5432/db_ditalk?sslmode=disable
export GOOSE_DRIVER
export GOOSE_DBSTRING

.PHONY: migrate-up migrate-down migrate-status migrate-reset migrate-create run build

migrate-up:
	goose -dir migrations up

migrate-down:
	goose -dir migrations down

migrate-status:
	goose -dir migrations status

migrate-reset:
	goose -dir migrations reset

# usage: make migrate-create name=add_something
migrate-create:
	goose -dir migrations -s create $(name) sql

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api
