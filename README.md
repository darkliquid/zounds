# zounds

`zounds` is a Go toolkit and CLI for scanning, analyzing, tagging, clustering, deduplicating, transforming, and browsing sound-sample libraries.

The codebase is library-first: the reusable logic lives under `pkg/`, and the `zounds` CLI layers workflows on top of it.

## Current capabilities

- scan directories for common audio formats
- decode WAV, AIFF, FLAC, MP3, and OGG/Vorbis
- analyze metadata, spectrum, spectral contrast/bandwidth, chroma, tonnetz, HPSS ratios, tempo, pitch, key, MFCC, loudness, ADSR-style dynamics, formants, splice points, waveform shape, harmonicity, quality metrics, and perceptual fingerprints
- derive tags from paths, embedded metadata, configurable expr-based rule files, local feature-vector classification, optional CLAP classifier services, and optional OpenAI-compatible LLM inference
- find exact and perceptual duplicates
- compute similarity, k-means clusters, DBSCAN clusters, and 2D PCA/t-SNE projections
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
zounds similar
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
zounds tag --auto --rule-file ./rules.json --clap-model-dir ./models/clap
zounds similar --threshold 0.85 --limit 20 ./Samples/Kicks/punch.wav ./Samples/Snares/crack.wav
zounds cluster --method kmeans --k 12 --projection tsne
zounds dedup --perceptual --threshold 8
zounds rename --template '{{join .Tags "_"}}_{{slug .Stem}}.{{.Extension}}' --dry-run
zounds serve --port 8080
```

By default, the CLI stores its SQLite database at `$XDG_DATA_HOME/zounds/zounds.db`, falling back to `~/.local/share/zounds/zounds.db` when `XDG_DATA_HOME` is unset. Use `--db` to override it.

For local CLAP tagging, download `audio_model.onnx`, `text_model.onnx`, and `tokenizer.json` from [Xenova/clap-htsat-unfused](https://huggingface.co/Xenova/clap-htsat-unfused), place them in a directory such as `./models/clap`, and pass that directory with `--clap-model-dir`. You also need the ONNX Runtime shared library available on your system, or provide its path with `--clap-lib`.

## Tagging rules

`zounds tag --auto` can load a JSON rule file with `--rule-file`. Rules are evaluated with the [`expr`](https://expr-lang.org/) language against this environment:

- `Metrics["name"]` for numeric analysis outputs such as `frequency_hz`, `spectral_flux`, `loop_confidence`, or `spectral_centroid_hz`
- `Attributes["name"]` for string attributes such as `key`, `note_name`, `format`, or embedded metadata values
- `Sample.Path`, `Sample.RelativePath`, `Sample.FileName`, `Sample.Extension`, `Sample.Format`, and `Sample.SizeBytes` for explicit file-aware rules

Each rule defines a tag, an expression, and optional `confidence` and `source` fields:

```json
{
  "rules": [
    {
      "tag": "cyberpunk",
      "expr": "Metrics[\"spectral_flux\"] > 0.1 && Attributes[\"mode\"] == \"minor\"",
      "confidence": 0.9
    }
  ]
}
```

Use it during auto-tagging:

```bash
zounds scan ~/Samples
zounds tag --auto --rule-file ./rules.json
```

Example rule file with both analysis-driven and path-aware tags:

```json
{
  "rules": [
    {
      "tag": "sub",
      "expr": "Metrics[\"frequency_hz\"] > 0 && Metrics[\"frequency_hz\"] < 120 && Metrics[\"confidence\"] > 0.5",
      "confidence": 0.75
    },
    {
      "tag": "didgeridoo",
      "expr": "Sample.RelativePath contains \"Didgeridoo Loops\"",
      "source": "rules"
    },
    {
      "tag": "oneshot",
      "expr": "\"loop_confidence\" in Metrics && Metrics[\"loop_confidence\"] < 0.35"
    }
  ]
}
```

This is the recommended way to tag from folder or filename conventions. Generic path token extraction is noisy, so keep path-based tagging explicit in rules instead of relying on automatic path tokenization.

## Library layout

```text
cmd/zounds/commands  Cobra command implementations
pkg/analysis         reusable analyzers and feature-vector building
pkg/audio            PCM buffer, codecs, registry, playback
pkg/cluster          similarity, k-means, DBSCAN, PCA and t-SNE projection
pkg/convert          resampling, channel conversion, transcoding, normalization
pkg/core             shared domain types and interfaces
pkg/db               SQLite connection, migrations, repository helpers
pkg/dedup            exact and perceptual dedup logic
pkg/rename           template rename and organization planning
pkg/scanner          recursive audio file discovery
pkg/tags             path, metadata, rule, local, CLAP, and LLM taggers
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
GET /api/clusters?projection=tsne
GET /api/clusters/:id
```

## Development

```bash
make tidy
make fmt
make test
```
