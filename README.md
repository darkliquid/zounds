# zounds

`zounds` is a Go toolkit and CLI for scanning, analyzing, tagging, clustering, deduplicating, transforming, and browsing sound-sample libraries.

The codebase is library-first: the reusable logic lives under `pkg/`, and the `zounds` CLI layers workflows on top of it.

## Current capabilities

- scan directories for common audio formats
- decode WAV, AIFF, FLAC, MP3, and OGG/Vorbis
- analyze metadata, spectrum, tempo, pitch, key, MFCC, loudness, dynamics, waveform shape, and perceptual fingerprints
- derive tags from paths, embedded metadata, rule-based heuristics, local feature-vector classification, and optional OpenAI-compatible LLM inference
- find exact and perceptual duplicates
- compute similarity, k-means clusters, DBSCAN clusters, and 2D projections
- convert sample rate, channel layout, output format, and loudness
- rename and organization-plan samples from templates, tags, and clusters
- browse and preview indexed samples from the CLI
- serve an embedded web UI with API endpoints, tag cloud, sample browser, preview playback, and cluster bubble view

## CLI

Implemented commands:

```text
zounds scan
zounds analyze
zounds tag
zounds cluster
zounds dedup
zounds convert
zounds rename
zounds export
zounds info
zounds play
zounds browse
zounds serve
```

Examples:

```bash
zounds scan ~/Samples
zounds analyze --all
zounds tag --auto
zounds cluster --method kmeans --k 12
zounds dedup --perceptual --threshold 8
zounds rename --template '{{join .Tags "_"}}_{{slug .Stem}}.{{.Extension}}' --dry-run
zounds serve --port 8080
```

## Library layout

```text
cmd/zounds/commands  Cobra command implementations
pkg/analysis         reusable analyzers and feature-vector building
pkg/audio            PCM buffer, codecs, registry, playback
pkg/cluster          similarity, k-means, DBSCAN, 2D projection
pkg/convert          resampling, channel conversion, transcoding, normalization
pkg/core             shared domain types and interfaces
pkg/db               SQLite connection, migrations, repository helpers
pkg/dedup            exact and perceptual dedup logic
pkg/rename           template rename and organization planning
pkg/scanner          recursive audio file discovery
pkg/tags             path, metadata, rule, local, and LLM taggers
web                  embedded HTTP server and static web UI
```

## Web UI

`zounds serve` starts the embedded HTTP server and frontend.

Available API routes include:

```text
GET /api/health
GET /api/samples
GET /api/samples?q=...
GET /api/samples?tag=...
GET /api/samples/:id
GET /api/samples/:id/audio
GET /api/tags
GET /api/tags/:name/samples
GET /api/clusters
GET /api/clusters/:id
```

## Development

```bash
make tidy
make fmt
make test
```
