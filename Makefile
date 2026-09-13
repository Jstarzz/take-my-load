.PHONY: test fmt run-controller run-worker compose-up compose-down

test:
	go test ./...
	cargo test --manifest-path engine/blast/Cargo.toml

fmt:
	gofmt -w $$(find cmd internal -name '*.go')
	cargo fmt --manifest-path engine/blast/Cargo.toml

run-controller:
	go run ./cmd/controller

run-worker:
	TML_WORKER_ID=dev-worker TML_WORKER_NAME='Dev Worker' go run ./cmd/worker

compose-up:
	docker compose up --build

compose-down:
	docker compose down
