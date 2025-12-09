# Juno Pool (scaffold)

Early scaffold for the Juno Cash mining pool. Focus is stratum front-end, job management, share tracking, accounting, and payouts.

## Quickstart
1) Copy `config.example.yaml` to `config.yaml` and fill values (TLS cert/key, node RPC, NATS, Postgres, fees).
2) Build and run:
   ```bash
   go mod tidy
   go run ./cmd/stratumd --config config.yaml
   ```
3) TLS is required; provide cert/key paths. The server currently accepts connections and stubs protocol handling.

## Layout
- `cmd/stratumd`: entrypoint for the stratum server.
- `internal/config`: config load/validate.
- `internal/stratum`: listener skeleton; TODO handshake/share handling.
- `internal/job`: block template source interface.
- `internal/share`: share validator interface.

## Next work items
- Implement Stratum V1 protocol handling (login, set_difficulty, submit).
- Wire template source to Juno node RPC; cache templates and push updates.
- Add vardiff controller and share accounting pipeline (NATS -> accounting service).
- Metrics and structured logging.
- Tests for config validation and listener lifecycle.
