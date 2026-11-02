# AGENTS.md — Zounds Architecture & Development Guide

`zounds` is a library-first Go toolkit (`pkg/`), CLI (`cmd/zounds`), and embedded web application (`web/`) for intelligent categorization, musical/acoustic feature extraction, auto-tagging, deduplication, multi-metric similarity search, clustering, and organization of sound-sample libraries.

## 1. Product Vision & Core Capabilities

Every architectural and algorithmic choice serves high-precision management of large real-world sample libraries (10k–500k+ one-shots, loops, stems, FX, and multisamples):

1. **Sample Intelligence & Extraction**: Extract container and musical attributes—duration/length, format, sample rate, bit depth, channels, embedded DAW/pack metadata (ID3, Vorbis, RIFF `smpl`/`acid`/`bext`/`LIST`, AIFF Apple Loop `basc`), tempo (BPM + confidence), musical key/scale, **chord** (triads, 7ths, inversions, confidence), fundamental pitch/MIDI note + cents offset, **one-shot vs loop** classification, ITU-R BS.1770-4 LUFS/dBTP loudness, ADSR dynamics, spectral/timbral descriptors, and audio quality defects (clipping, DC offset, silence, fake-HQ transcodes).
2. **Faceted Auto-Tagging**: Combine embedded metadata, explicit user-authored `expr` rules (`--rule-file`, never implicit default rules), zero-shot audio-language embeddings (local ONNX CLAP), similarity-based k-NN propagation (`LocalModelTagger`), and optional LLM refinement into a provenance-tracked, **namespaced** tag taxonomy (`facet:value`, e.g. `instrument:kick`, `role:lead`, `character:dark`, `articulation:oneshot`, `articulation:loop`, `genre:techno`).
3. **Deduplication**: Detect byte-exact duplicates (`SHA-256` with size pre-filtering) and perceptual near-duplicates across format transcodes, gain changes, and padding, selecting canonical keepers by audio fidelity.
4. **Multi-Metric Similarity & Clustering**: Support similarity search, clustering (`kmeans`, `dbscan`, HDBSCAN), and 2D projection (`pca`, `tsne`) across normalized, selectable metric spaces—semantic (`clap` 512-D embeddings), timbral (MFCC/spectral), harmonic (chroma/tonnetz/key/chord), rhythmic (BPM/onset envelope), and perceptual fingerprints.
5. **Non-Destructive Organization & Exploration**: Safe template renaming, tag/cluster folder organization, format/rate/loudness conversion, terminal preview (`play`, `browse`), and a responsive embedded Web UI (`serve`).

---

## 2. Build, Test, Lint & Runtime Environment

- **Toolchain**: Go `1.26` (`module github.com/darkliquid/zounds`).
- **Commands**:
  ```bash
  make test              # go test ./...
  make build             # go build ./...
  make fmt               # gofmt -w on all .go files
  make tidy              # go mod tidy
  golangci-lint run      # CI runs golangci-lint v2 (.github/workflows/ci.yml)
  ```
- **SQLite Database**: `$XDG_DATA_HOME/zounds/zounds.db` (when `XDG_DATA_HOME` is absolute), else `~/.local/share/zounds/zounds.db`, overridable via `--db`. Uses pure-Go `modernc.org/sqlite` (`SetMaxOpenConns(1)`, `foreign_keys(1)`, `busy_timeout(5000)`).
- **Local Untracked Assets**: `onnx/`, `onnxruntime.so`, `.db` files, and `./zounds` binaries may exist locally—**never commit binary `.onnx` models, `onnxruntime.so`, databases, or compiled binaries**. CLAP expects `audio_model.onnx`, `text_model.onnx`, and `tokenizer.json` from `Xenova/clap-htsat-unfused`.
- **`CGO_ENABLED=0` Cross-Compilation Rule**:
  - `.github/workflows/release.yml` builds release binaries with `CGO_ENABLED=0` across `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`, and `windows/amd64`.
  - `modernc.org/sqlite` and `ebitengine/oto/v3` (`purego`) compile under `CGO_ENABLED=0`, **but `github.com/yalue/onnxruntime_go` currently requires CGO** and breaks `CGO_ENABLED=0` builds. Migrate `pkg/clap`'s ONNX Runtime C API bindings (`OrtGetApiBase`) to `github.com/ebitengine/purego` so `CGO_ENABLED=0 go build ./cmd/zounds` succeeds across all targets while keeping full local CLAP inference.

---

## 3. Codebase Map & Dependency Rules

### Strict Layering Invariant
- `pkg/core` defines shared domain types (`Sample`, `Tag`, `FeatureVector`, `Cluster`, `AnalysisResult`) and interfaces (`Analyzer`, `Tagger`, `Clusterer`) with **zero internal dependencies**.
- **Keep pipeline orchestration in `pkg/*`, never in `cmd/zounds/commands` or `web`.** Shared workflows (multi-analyzer runs with cached decode, auto-tagging, multi-reference similarity search, clustering + projection persistence, filtered sample/tag queries, collision-checked rename/organize, conversion chains) belong in `pkg/*` so CLI, HTTP API, and library callers share one implementation.

### Package Directory Reference

| Package | Responsibility & Key Types | Current Status & Notes |
|---|---|---|
| `pkg/core` | Domain structs, interfaces, `DetectFormatFromExtension`, `NormalizeTagName`. | Structs lack `json:"..."` tags (PascalCase JSON in `web`/`export`). |
| `pkg/db` | SQLite `Open`, `Migrate` (`migrations/*.sql`), `Repository` CRUD. | `MaxOpenConns(1)`; tracks `schema_migrations` and persists `sample_analyses`/`sample_projections`. `perceptual_hashes` CRUD remains missing. |
| `pkg/audio` | `PCMBuffer` (interleaved `[]float64` in `[-1, 1]`), `Decoder`/`Encoder`, `Registry`, `Playback` (`oto/v3`), `codecs.NewRegistry()`. | Decode: `wav`, `aiff`, `flac`, `mp3`, `ogg`. Encode: `wav`, `aiff` only. **No `aac`/`m4a` codec** despite scanner indexing them. |
| `pkg/scanner` | Concurrent discovery (`Scanner.Scan`): `WalkDir` + `os.Stat` workers, path-sorted output. | Indexes `.wav`, `.aif/.aiff`, `.mp3`, `.flac`, `.ogg/.oga`, `.aac`, `.m4a`. No mtime skip or deleted-file pruning. |
| `pkg/analysis` | 16 `core.Analyzer` implementations (`metadata`, `spectral`, `key`, `beat`, `pitch`, `loudness`, `dynamics`, `hpss`, `quality`, `harmonics`, `formants`, `splice`, `mfcc`, `loop`, `perceptual_hash`, `waveform`), `ContextWithDecodedSample`, 95-D `FeatureVectorBuilder`. | `analyze` runs 14 analyzers concurrently (omits `perceptual_hash`, `waveform`). **No chord analyzer yet.** |
| `pkg/clap` | Local ONNX Runtime zero-shot classifier (`Model`, 48 kHz 64-bin log-mel `FeatureExtractor`, RoBERTa `BPETokenizer`). | Ephemeral `ClassifyAudio` only; does not expose or persist 512-D embeddings. Has preprocessing/tokenizer bugs (§4). |
| `pkg/tags` | `MetadataTagger`, `RuleTagger` (`expr`), `LocalCLAPTagger`, `PathTagger`, `LocalModelTagger` (weighted k-NN), `LLMTagger` (OpenAI-compatible API). | Only `MetadataTagger`, `RuleTagger` (`--rule-file`), and `LocalCLAPTagger` (`--clap-model-dir`) are wired into `tag --auto`. |
| `pkg/dedup` | `ExactFinder` (size pre-bucket + worker `SHA-256`), `PerceptualFinder` (Hamming Union-Find), `PlanCull`. | Perceptual dedup recomputes hashes sequentially from disk on every run instead of using `perceptual_hashes`. |
| `pkg/cluster` | `CosineSimilarity`, `EuclideanDistance`, `Pairwise`, `NearestNeighbors`, `KMeans`, `DBSCAN`, `Project2D` (SVD PCA), `ProjectTSNE2D`. | Operates on raw unstandardized vectors. `DBSCAN` is implemented here but rejected by CLI `cluster`. |
| `pkg/convert` | `ResampleLinear`, `ConvertChannels` (`1 <-> 2`), `Normalize` (`peak`, `rms`, `lufs`), `TranscodeFile`. | `ResampleLinear` lacks anti-aliasing; `NormalizeLUFS` currently aliases unweighted RMS. |
| `pkg/rename` | `BuildTemplateData`, `RenderTemplate` (`join`, `slug`, `lower`, `upper`), `OrganizeByTags`, `OrganizeByCluster`. | `OrganizeBy*` unwired; CLI `rename` lacks collision checks and passes empty `Attributes`/`Metadata`. |
| `web` | Embedded SPA (`index.html`, `app.js`, `style.css`) + HTTP server (`NewServer`, `ListenAndServe`). | Tag cloud, search, inline tag editing, `<audio>` streaming, SVG cluster map. Primary sample/cluster queries are batched and projections are persisted; `/api/tags/:name/samples` still uses per-sample tag queries. |
| `cmd/zounds` | Re-entrant Cobra CLI (`NewRootCommand`) with 13 commands: `scan`, `analyze`, `info`, `tag`, `similar`, `cluster`, `dedup`, `convert`, `rename`, `export`, `play`, `browse`, `serve`. | `main.go` lacks `signal.NotifyContext`; `-j`/`--concurrency` is ignored outside `scan` and `dedup --exact`. |

---

## 4. Known Bugs, Architectural Gaps & Landmines

Account for these verified defects before extending affected subsystems:

### A. Build, Test & CI Breakages on `HEAD`
1. **Failing CLI Tests (`cmd/zounds/commands/tag_test.go:138`, `integration_test.go:53`)**: Commit `85f3645` unwired `PathTagger` and `5edb487` intentionally made `RuleTagger` strictly explicit (`--rule-file` required; no implicit default rules). Update those CLI tests to pass explicit rule fixtures/tag sources rather than restoring implicit default rules.
2. **Broken `CGO_ENABLED=0` Release Build**: `pkg/clap/onnx.go` imports CGO-based `yalue/onnxruntime_go`, breaking `.github/workflows/release.yml` (replace with `ebitengine/purego`).
3. **Linter Warning (`pkg/clap/model.go:64`)**: `reflect.Ptr` triggers `govet` deprecation in `golangci-lint` (use `reflect.Pointer`).

### B. Data-Loss, Filesystem & Codec Hazards
4. **`zounds rename` Silent File Overwrite (`cmd/zounds/commands/rename.go:66`)**: Runs `os.Rename(sample.Path, destination)` with **zero destination collision check**, zero batch duplicate-target preflight, dot-in-stem extension loss (`kick.v2.wav`), and no path-traversal stripping in `slug()`. Colliding templates permanently overwrite files on POSIX before SQLite throws `UNIQUE constraint failed: samples.path`.
5. **32-Bit IEEE Float WAV Corruption & 8-Bit PCM Bug (`pkg/audio/wav/codec.go`, `pkg/audio/pcm.go`)**: `go-audio/wav` ignores `WavAudioFormat == 3` (IEEE float, common in DAW/Splice exports) and misreads float32 bytes as signed `int32` (`±0.5` garbage). `go-audio/wav` and `go-audio/aiff` also decode 8-bit PCM as unsigned `[0..255]`, which `IntBufferToPCM` scales as signed `[-128..127]`.
6. **OGG Float Overshoot & `Validate()` (`pkg/audio/ogg/codec.go:39`, `pkg/audio/buffer.go:61`)**: Vorbis decoding routinely produces peaks slightly over `±1.0` (`1.003`), failing `PCMBuffer.Validate()` in playback, resampling, normalization, perceptual hashing, and transcoding. `Validate()` also misses `math.NaN()`.
7. **Scanner/Codec Mismatch (`pkg/scanner/scanner.go:28-29`)**: `scanner` indexes `.aac` and `.m4a` with no registered decoder, causing `analyze --all` and `dedup --perceptual` to abort instead of logging a warning and skipping undecodable samples.
8. **Playback Rate Lock-In & Goroutine Leaks (`pkg/audio/playback.go:104-177`, `pkg/scanner/scanner.go:76-138`, `pkg/dedup/exact.go:59-131`)**: `oto.NewContext` locks onto the first played sample's rate/channels and errors on subsequent mismatches instead of resampling/upmixing, while leaking context-wait goroutines; `scanner` and `exact` leak worker goroutines on early error due to unbuffered channels without `context.WithCancel`.

### C. Persistence & Schema Bottlenecks (`pkg/db`)
9. **Discarded Analysis Results (`cmd/zounds/commands/analyze.go:58`)**: `analyze` discards `results` (`_ = results`) and persists only the 95-float `"analysis"` `FeatureVector`. String attributes (`key`, `mode`, `note_name`, `loop_class`, `embedded.*`) and named metrics are lost, leaving `rename` `.Attributes`/`.Metadata` empty and forcing `tag --auto` to re-decode files.
10. **Per-Sample Tag Provenance (Resolved Guardrail)**: `source` and `confidence` live on `sample_tags(sample_id, tag_id)` via `0002_rich_analysis_and_provenance.sql`; `ReplaceSampleTags` preserves provenance for retained tags and assigns explicit provenance to new tags.
11. **Versioned Migration Runner (Resolved)**: `Migrate()` records applied files in `schema_migrations`; add new schema changes as new numbered `migrations/*.sql` files rather than editing applied migrations.
12. **Orphaned `perceptual_hashes` & Projection Fallback**: `perceptual_hashes` has zero Go callers. `sample_projections` stores 2D coordinates; `web` recomputes projections only when persisted coordinates are absent, and `/api/tags/:name/samples` still executes N+1 tag queries.

### D. Feature Space, Similarity, Clustering & Dedup Flaws
13. **Unstandardized 95-D Vector Dominated by Container Metadata (`pkg/analysis/features.go:13-109`)**: Mixes `bitrate` (`~1.4e6`), `sample_rate` (`44100`), and Hz values unscaled with `[0, 1]` ratios (`chroma`, `tonnetz`, `loop_confidence`). `bitrate` and `sample_rate` dominate `>99.99%` of vector norm/distance in `similar`, `KMeans`, `DBSCAN`, `PCA`, `t-SNE`, and `LocalModelTagger`.
14. **t-SNE Exponential Underflow & Small-N Bug (`pkg/cluster/tsne.go:197-220`)**: `perplexityRow` starts at `beta = 1.0`; when squared Euclidean distance `> 745` (`dist > 27.3`), `math.Exp(-distance * beta)` underflows to `0` and `if sum == 0 { break }` exits with all-zero affinities instead of halving `beta`. Also fails to clamp `Perplexity < float64(N-1)`.
15. **1D-FFT `"spectral-8x8"` Perceptual Hash (`pkg/analysis/hash.go:95-184`, `pkg/dedup/perceptual.go`)**: Despite `windows = 8`, runs **one 1D FFT** on only the first `1.024s` (`samples[:8192]` at 8 kHz via aliased `ResampleLinear`), ignoring time order and audio after `1.024s`. `PerceptualFinder` suffers from Union-Find transitive chaining and `PlanPerceptualCull` ignores audio quality (`WAV` vs `MP3`).

### E. CLAP, Tagging & DSP Extraction Gaps
16. **CLAP Preprocessing & Tokenizer Bugs (`pkg/clap/tokenizer.go:210`, `pkg/clap/melspec.go:14-21, 102-192`)**:
    - `BPETokenizer.Encode` pads trailing `input_ids` with `0` (`<s>` BOS in RoBERTa) instead of `1` (`<pad>`).
    - `melspec.go` uses `HopLength = 320` instead of `Xenova/clap-htsat-unfused`'s `480`, min-max scales log-mel dB to `[0, 1]` instead of feeding raw dB to HTSAT's input `BatchNorm2d`, and zero-pads short one-shots across 10s instead of `repeatpad`.
    - `FeatureExtractor` mutates shared `fourier.FFT` scratch slices without a mutex (data race), re-runs the 479 MB RoBERTa text encoder per label per sample without caching, and discards 512-D audio embeddings instead of persisting them to SQLite.
17. **Incomplete `tag --auto` Analyzer Pipeline (`cmd/zounds/commands/tag.go:95-172`)**: Runs only 6 of 14 analyzers sequentially **without** `ContextWithDecodedSample` (6–7x redundant disk decodes) and omits `LoopAnalyzer`, `BeatAnalyzer`, `LoudnessAnalyzer`, etc.—breaking `loop_confidence` (documented in `README.md`) and `tempo_bpm` rules.
18. **DSP Extraction Gaps vs Product Requirements**:
    - **Chord Detection**: Completely missing (`KeyAnalyzer` only matches 24 Krumhansl-Kessler key profiles and mislabels single notes as major keys due to 3rd/5th overtones).
    - **WAV/AIFF Embedded Sample Metadata**: `dhowden/tag` ignores WAV `smpl`, `acid`, `bext`, `LIST/INFO`, RIFF `id3 `, and AIFF Apple Loop `basc` chunks.
    - **Fake LUFS (`pkg/analysis/loudness.go:115`, `pkg/convert/normalize.go:40`)**: `integrated_lufs` and `NormalizeLUFS` alias unweighted `rmsDBFS` (missing BS.1770 K-weighting, channel summing, and silence gating).
    - **Tempo, Pitch, Loop & Quality Weaknesses**: `BeatAnalyzer` uses unnormalized autocorrelation (200 BPM bias, emits BPM on one-shots with no confidence); `PitchAnalyzer` lacks parabolic lag interpolation (`~40 cent` error at 1 kHz), cuts off `< 50 Hz` (misses 808s/sub-bass), and emits notes on noise; `LoopAnalyzer` tests `lag = 1..20` smooth RMS autocorrelation (false-positive `"loop"` on short stabs) and 46 ms head/tail spectra (false-negative `"oneshot"` on gapped drum loops); `QualityAnalyzer` checks clipping *after* mono downmix; `DynamicsAnalyzer` counts `|x[n]-x[n-1]| >= 0.2` (flagging HF sine waves as transients); `MFCCAnalyzer` and `FormantAnalyzer` scale directly with raw `SampleRate`.

---

## 5. Engineering Standards & Directives

1. **Preserve `CGO_ENABLED=0` Portability via `purego`**: Every package—including `pkg/clap` ONNX Runtime bindings—must build and pass tests under `CGO_ENABLED=0`. Use `github.com/ebitengine/purego` (never CGO) for dynamic shared-library bridges.
2. **Explicit Rules & Namespaced Tags (`facet:value`)**:
   - Keep `RuleTagger` strictly explicit (`--rule-file` opt-in; no hidden built-in heuristic rules).
   - Represent faceted tags using `facet:value` namespacing (`instrument:kick`, `role:pad`, `character:metallic`, `articulation:oneshot`, `genre:dnb`). `core.NormalizeTagName` must preserve `:` and trim whitespace around it (`"instrument : sub_bass"` -> `"instrument:sub bass"`), while `pkg/rename` `slug()` must sanitize `:` (and `/`, `\`, `..`) for cross-platform filesystem paths.
3. **DSP Correctness & Sample-Rate Invariance**:
   - Specify window lengths, hop sizes, filterbank bounds (e.g. Mel `20–16000 Hz`), and LPC rates (`10–11 kHz` downsampled) in **seconds / Hz**, never raw sample counts.
   - Use anti-aliased windowed-sinc (polyphase FIR) resampling whenever downsampling.
   - Gate categorical musical predictions (`key`, `chord`, `note_name`, `tempo_bpm`) on confidence thresholds—unpitched percussion/noise must emit empty/null pitch, key, and chord attributes.
   - Run per-channel checks (clipping, DC offset, phase correlation, BS.1770 LUFS, 4x True Peak) **before** mono downmixing.
   - Share per-sample intermediates via an immutable `AnalysisContext` (decoded `PCMBuffer`, mono `[]float64`, cached multi-resolution STFTs).
   - Batch pipelines (`analyze --all`, `tag --auto`, `dedup --perceptual`) must **skip undecodable samples** (e.g. `.aac`/`.m4a` or corrupt headers) with a warning rather than aborting the batch.
4. **Separate & Standardize Feature Spaces**:
   - **Exclude container metadata** (`sample_rate`, `channels`, `bit_depth`, `bitrate`, `size_bytes`) from acoustic similarity/clustering vectors; filter container properties in SQL.
   - Persist distinct `feature_vectors` namespaces: `"acoustic"` (standardized via dataset z-score/IQR scaling), `"clap"` (L2-normalized 512-D semantic embeddings), and sub-space views (`timbre`, `harmonic`, `rhythm`) for `--metric` selection.
5. **SQLite & Filesystem Safety**:
   - Track migrations in `schema_migrations(version TEXT PRIMARY KEY, applied_at TEXT)`.
   - Wrap bulk writes (`scan`, `analyze`, `tag`, `cluster`) in explicit transactions (`BEGIN IMMEDIATE ... COMMIT`) and replace N+1 loops with batch `JOIN` / `IN (...)` queries.
   - Never mutate or overwrite audio files without batch collision preflight, path sanitization, and atomic temp-file write + rename.
6. **Verification Before Completion**:
   - Test every change with deterministic synthetic audio fixtures in `t.TempDir()` and verify all three pass:
     1. `go test ./...`
     2. `CGO_ENABLED=0 go build ./...`
     3. `golangci-lint run`

---

## 6. Development Roadmap

The phased roadmap (Phase 0: Stabilization & Data Safety through Phase 5: Web Workstation & Rich Export) is tracked as beads in the `bd` issue tracker (`bd ready`, `bd ready --priority 0`). Phase epics are the `Phase N: ...` issues; each carries the relevant §4 defect references, priority (P0–P4), and dependency edges between tasks. The roadmap lives only in beads — keep it there rather than re-adding phase lists here.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:46cd31e7 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/core-concepts/sync-concepts.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   bd dolt push
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->

<!-- BEGIN BEADS CODEX SETUP: generated by bd setup codex -->
## Beads Issue Tracker

Use Beads (`bd`) for durable task tracking in repositories that include it. Use the `beads` skill at `.agents/skills/beads/SKILL.md` (project install) or `~/.agents/skills/beads/SKILL.md` (global install) for Beads workflow guidance, then use the `bd` CLI for issue operations.

### Quick Reference

```bash
bd ready                # Find available work
bd show <id>            # View issue details
bd update <id> --claim  # Claim work
bd close <id>           # Complete work
bd prime                # Refresh Beads context
```

### Rules

- Use `bd` for all task tracking; do not create markdown TODO lists.
- Run `bd prime` when Beads context is missing or stale. Codex 0.129.0+ can load Beads context automatically through native hooks; use `/hooks` to inspect or toggle them.
- Keep persistent project memory in Beads via `bd remember`; do not create ad hoc memory files.

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/core-concepts/sync-concepts.md for details and anti-patterns.
<!-- END BEADS CODEX SETUP -->
