# zounds

`zounds` is a Go toolkit for scanning and organizing large sound-sample libraries.

Phase 1 establishes the project foundation:

- Go module at `github.com/darkliquid/zounds`
- core domain types and interfaces in `pkg/core`
- SQLite-backed metadata store with embedded migrations in `pkg/db`
- concurrent directory scanner for supported audio file extensions in `pkg/scanner`

Phase 2 has started with shared audio buffer/codec abstractions, playback support, and pure-Go codecs for WAV, AIFF, FLAC, MP3, and OGG/Vorbis.

Phase 3 has started with reusable metadata, spectral, beat, pitch, key, MFCC, loudness, and dynamics extraction, plus consolidated feature-vector building, including embedded file metadata (for example ID3/Vorbis/FLAC tags), and the tagging layer now includes path-based tag extraction.

## Current layout

```text
cmd/zounds         CLI entrypoint placeholder
cmd/zounds/commands Cobra root command and command tree scaffold
pkg/core           shared domain types and interfaces
pkg/audio          PCM buffer, codec interfaces, registries
pkg/analysis       reusable analyzers, starting with metadata extraction
pkg/cluster        similarity and clustering building blocks
pkg/convert        reusable audio transformation helpers (channels, resampling, transcoding, normalization)
pkg/db             SQLite connection, migrations, repository helpers
pkg/dedup          exact/perceptual deduplication building blocks
pkg/rename         template-based rename/reorganization helpers
pkg/scanner        recursive file discovery for audio samples
pkg/tags           taggers, starting with path-derived tags
```

## Development

```bash
make tidy
make fmt
make test
```
