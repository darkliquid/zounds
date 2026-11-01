-- Rich per-sample analysis persistence and per-sample tag provenance.

-- 1. sample_analyses: one row per sample holding the flattened analysis
--    attributes and metrics from the analyzer pipeline.
CREATE TABLE IF NOT EXISTS sample_analyses (
    sample_id INTEGER PRIMARY KEY,
    duration_seconds REAL,
    sample_rate INTEGER,
    channels INTEGER,
    bit_depth INTEGER,
    bpm REAL,
    bpm_confidence REAL,
    key_tonic TEXT,
    key_mode TEXT,
    key_confidence REAL,
    chord TEXT,
    chord_confidence REAL,
    pitch_hz REAL,
    midi_note INTEGER,
    cents_offset REAL,
    pitch_confidence REAL,
    loop_class TEXT,
    loop_confidence REAL,
    lufs_integrated REAL,
    true_peak_dbtp REAL,
    analyzed_modified_at TEXT,
    metrics_json TEXT NOT NULL DEFAULT '{}',
    attributes_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (sample_id) REFERENCES samples(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sample_analyses_duration ON sample_analyses(duration_seconds);
CREATE INDEX IF NOT EXISTS idx_sample_analyses_bpm ON sample_analyses(bpm);
CREATE INDEX IF NOT EXISTS idx_sample_analyses_key ON sample_analyses(key_tonic, key_mode);
CREATE INDEX IF NOT EXISTS idx_sample_analyses_chord ON sample_analyses(chord);
CREATE INDEX IF NOT EXISTS idx_sample_analyses_loop_class ON sample_analyses(loop_class);
CREATE INDEX IF NOT EXISTS idx_sample_analyses_pitch_hz ON sample_analyses(pitch_hz);
CREATE INDEX IF NOT EXISTS idx_sample_analyses_analyzed_modified_at ON sample_analyses(analyzed_modified_at);

-- 2. Per-sample tag provenance: source and confidence move from the global
--    tags table to sample_tags so attaching a tag to one sample no longer
--    overwrites the provenance recorded for every other sample.
ALTER TABLE sample_tags ADD COLUMN source TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE sample_tags ADD COLUMN confidence REAL NOT NULL DEFAULT 1.0;

UPDATE sample_tags
SET
    source = (
        SELECT t.source FROM tags t WHERE t.id = sample_tags.tag_id
    ),
    confidence = (
        SELECT t.confidence FROM tags t WHERE t.id = sample_tags.tag_id
    );

ALTER TABLE tags DROP COLUMN source;
ALTER TABLE tags DROP COLUMN confidence;

-- 3. feature_vectors: enforce one vector per (sample, namespace) so
--    re-analysis replaces instead of accumulating rows. Legacy duplicates
--    are pruned keeping the newest row before the unique index is created.
DELETE FROM feature_vectors
WHERE id NOT IN (
    SELECT MAX(id) FROM feature_vectors GROUP BY sample_id, namespace
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_feature_vectors_sample_namespace
    ON feature_vectors(sample_id, namespace);

-- 4. sample_projections: persisted 2D projection coordinates so the web UI
--    and CLI can serve precomputed maps instead of recomputing t-SNE.
CREATE TABLE IF NOT EXISTS sample_projections (
    sample_id INTEGER NOT NULL,
    method TEXT NOT NULL,
    namespace TEXT NOT NULL,
    x REAL NOT NULL,
    y REAL NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (sample_id, method, namespace),
    FOREIGN KEY (sample_id) REFERENCES samples(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sample_projections_method_namespace
    ON sample_projections(method, namespace);
