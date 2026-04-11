# zounds

`zounds` is a Go toolkit for scanning and organizing large sound-sample libraries.

Phase 1 establishes the project foundation:

- Go module at `github.com/darkliquid/zounds`
- core domain types and interfaces in `pkg/core`
- SQLite-backed metadata store with embedded migrations in `pkg/db`
- concurrent directory scanner for supported audio file extensions in `pkg/scanner`

Phase 2 has started with the shared audio buffer/codec abstraction and an initial WAV codec implementation.

## Current layout

```text
cmd/zounds         CLI entrypoint placeholder
pkg/core           shared domain types and interfaces
pkg/audio          PCM buffer, codec interfaces, registries
pkg/db             SQLite connection, migrations, repository helpers
pkg/scanner        recursive file discovery for audio samples
```

## Development

```bash
make tidy
make fmt
make test
```
