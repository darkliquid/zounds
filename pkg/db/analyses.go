package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darkliquid/zounds/pkg/core"
)

func (r *Repository) UpsertSampleAnalysis(ctx context.Context, analysis core.SampleAnalysis) error {
	metricsJSON, err := marshalJSON(analysis.Metrics, "{}")
	if err != nil {
		return fmt.Errorf("marshal analysis metrics: %w", err)
	}
	attributesJSON, err := marshalJSON(analysis.Attributes, "{}")
	if err != nil {
		return fmt.Errorf("marshal analysis attributes: %w", err)
	}

	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO sample_analyses (
			sample_id,
			duration_seconds,
			sample_rate,
			channels,
			bit_depth,
			bpm,
			bpm_confidence,
			key_tonic,
			key_mode,
			key_confidence,
			chord,
			chord_confidence,
			pitch_hz,
			midi_note,
			cents_offset,
			pitch_confidence,
			loop_class,
			loop_confidence,
			lufs_integrated,
			true_peak_dbtp,
			analyzed_modified_at,
			metrics_json,
			attributes_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(sample_id) DO UPDATE SET
			duration_seconds = excluded.duration_seconds,
			sample_rate = excluded.sample_rate,
			channels = excluded.channels,
			bit_depth = excluded.bit_depth,
			bpm = excluded.bpm,
			bpm_confidence = excluded.bpm_confidence,
			key_tonic = excluded.key_tonic,
			key_mode = excluded.key_mode,
			key_confidence = excluded.key_confidence,
			chord = excluded.chord,
			chord_confidence = excluded.chord_confidence,
			pitch_hz = excluded.pitch_hz,
			midi_note = excluded.midi_note,
			cents_offset = excluded.cents_offset,
			pitch_confidence = excluded.pitch_confidence,
			loop_class = excluded.loop_class,
			loop_confidence = excluded.loop_confidence,
			lufs_integrated = excluded.lufs_integrated,
			true_peak_dbtp = excluded.true_peak_dbtp,
			analyzed_modified_at = excluded.analyzed_modified_at,
			metrics_json = excluded.metrics_json,
			attributes_json = excluded.attributes_json,
			updated_at = CURRENT_TIMESTAMP;
	`,
		analysis.SampleID,
		analysis.DurationSeconds,
		analysis.SampleRate,
		analysis.Channels,
		analysis.BitDepth,
		analysis.BPM,
		analysis.BPMConfidence,
		analysis.KeyTonic,
		analysis.KeyMode,
		analysis.KeyConfidence,
		analysis.Chord,
		analysis.ChordConfidence,
		analysis.PitchHz,
		analysis.MidiNote,
		analysis.CentsOffset,
		analysis.PitchConfidence,
		analysis.LoopClass,
		analysis.LoopConfidence,
		analysis.LUFSIntegrated,
		analysis.TruePeakDBTP,
		timeToValue(analysis.AnalyzedModifiedAt),
		metricsJSON,
		attributesJSON,
	); err != nil {
		return fmt.Errorf("upsert analysis for sample %d: %w", analysis.SampleID, err)
	}

	return nil
}

func (r *Repository) GetSampleAnalysis(ctx context.Context, sampleID int64) (core.SampleAnalysis, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			sample_id,
			duration_seconds,
			sample_rate,
			channels,
			bit_depth,
			bpm,
			bpm_confidence,
			key_tonic,
			key_mode,
			key_confidence,
			chord,
			chord_confidence,
			pitch_hz,
			midi_note,
			cents_offset,
			pitch_confidence,
			loop_class,
			loop_confidence,
			lufs_integrated,
			true_peak_dbtp,
			analyzed_modified_at,
			metrics_json,
			attributes_json,
			created_at,
			updated_at
		FROM sample_analyses
		WHERE sample_id = ?;
	`, sampleID)

	analysis, err := scanSampleAnalysis(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return core.SampleAnalysis{}, err
		}
		return core.SampleAnalysis{}, fmt.Errorf("get analysis for sample %d: %w", sampleID, err)
	}

	return analysis, nil
}

// ListSampleAnalysesBySampleIDs retrieves sample analyses for multiple sample IDs in chunks.
func (r *Repository) ListSampleAnalysesBySampleIDs(ctx context.Context, sampleIDs []int64) (map[int64]core.SampleAnalysis, error) {
	result := make(map[int64]core.SampleAnalysis, len(sampleIDs))
	if len(sampleIDs) == 0 {
		return result, nil
	}

	const batchSize = 500
	for i := 0; i < len(sampleIDs); i += batchSize {
		end := i + batchSize
		if end > len(sampleIDs) {
			end = len(sampleIDs)
		}
		chunk := sampleIDs[i:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for j, id := range chunk {
			placeholders[j] = "?"
			args[j] = id
		}

		query := fmt.Sprintf(`
			SELECT
				sample_id,
				duration_seconds,
				sample_rate,
				channels,
				bit_depth,
				bpm,
				bpm_confidence,
				key_tonic,
				key_mode,
				key_confidence,
				chord,
				chord_confidence,
				pitch_hz,
				midi_note,
				cents_offset,
				pitch_confidence,
				loop_class,
				loop_confidence,
				lufs_integrated,
				true_peak_dbtp,
				analyzed_modified_at,
				metrics_json,
				attributes_json,
				created_at,
				updated_at
			FROM sample_analyses
			WHERE sample_id IN (%s);
		`, strings.Join(placeholders, ","))

		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("list sample analyses by sample ids: %w", err)
		}

		for rows.Next() {
			analysis, err := scanSampleAnalysis(rows)
			if err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan sample analysis: %w", err)
			}
			result[analysis.SampleID] = analysis
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate sample analyses: %w", err)
		}
		_ = rows.Close()
	}

	return result, nil
}

func (r *Repository) DeleteSampleAnalysis(ctx context.Context, sampleID int64) error {
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM sample_analyses
		WHERE sample_id = ?;
	`, sampleID); err != nil {
		return fmt.Errorf("delete analysis for sample %d: %w", sampleID, err)
	}
	return nil
}

func (r *Repository) UpsertSampleProjection(ctx context.Context, projection core.SampleProjection) error {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO sample_projections (sample_id, method, namespace, x, y)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(sample_id, method, namespace) DO UPDATE SET
			x = excluded.x,
			y = excluded.y,
			updated_at = CURRENT_TIMESTAMP;
	`, projection.SampleID, projection.Method, projection.Namespace, projection.X, projection.Y); err != nil {
		return fmt.Errorf("upsert %s/%s projection for sample %d: %w", projection.Method, projection.Namespace, projection.SampleID, err)
	}
	return nil
}

func (r *Repository) GetSampleProjection(ctx context.Context, sampleID int64, method, namespace string) (core.SampleProjection, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT sample_id, method, namespace, x, y, created_at, updated_at
		FROM sample_projections
		WHERE sample_id = ? AND method = ? AND namespace = ?;
	`, sampleID, method, namespace)

	projection, err := scanSampleProjection(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return core.SampleProjection{}, err
		}
		return core.SampleProjection{}, fmt.Errorf("get %s/%s projection for sample %d: %w", method, namespace, sampleID, err)
	}

	return projection, nil
}

func (r *Repository) ListSampleProjections(ctx context.Context, method, namespace string) ([]core.SampleProjection, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sample_id, method, namespace, x, y, created_at, updated_at
		FROM sample_projections
		WHERE (? = '' OR method = ?) AND (? = '' OR namespace = ?)
		ORDER BY method, namespace, sample_id;
	`, method, method, namespace, namespace)
	if err != nil {
		return nil, fmt.Errorf("list sample projections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var projections []core.SampleProjection
	for rows.Next() {
		projection, err := scanSampleProjection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan sample projection: %w", err)
		}
		projections = append(projections, projection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sample projections: %w", err)
	}

	return projections, nil
}

func (r *Repository) UpsertPerceptualHash(ctx context.Context, hash core.PerceptualHash) error {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO perceptual_hashes (sample_id, algorithm, hash, hash_bits)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(sample_id, algorithm) DO UPDATE SET
			hash = excluded.hash,
			hash_bits = excluded.hash_bits;
	`, hash.SampleID, hash.Algorithm, hash.Hash, hash.HashBits); err != nil {
		return fmt.Errorf("upsert %s perceptual hash for sample %d: %w", hash.Algorithm, hash.SampleID, err)
	}
	return nil
}

func (r *Repository) GetPerceptualHash(ctx context.Context, sampleID int64, algorithm string) (core.PerceptualHash, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT sample_id, algorithm, hash, hash_bits, created_at
		FROM perceptual_hashes
		WHERE sample_id = ? AND algorithm = ?;
	`, sampleID, algorithm)

	hash, err := scanPerceptualHash(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return core.PerceptualHash{}, err
		}
		return core.PerceptualHash{}, fmt.Errorf("get %s perceptual hash for sample %d: %w", algorithm, sampleID, err)
	}

	return hash, nil
}

func (r *Repository) ListPerceptualHashes(ctx context.Context, algorithm string) ([]core.PerceptualHash, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sample_id, algorithm, hash, hash_bits, created_at
		FROM perceptual_hashes
		WHERE (? = '' OR algorithm = ?)
		ORDER BY algorithm, sample_id;
	`, algorithm, algorithm)
	if err != nil {
		return nil, fmt.Errorf("list perceptual hashes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var hashes []core.PerceptualHash
	for rows.Next() {
		hash, err := scanPerceptualHash(rows)
		if err != nil {
			return nil, fmt.Errorf("scan perceptual hash: %w", err)
		}
		hashes = append(hashes, hash)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate perceptual hashes: %w", err)
	}

	return hashes, nil
}

func marshalJSON(value any, emptyObject string) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if string(data) == "null" {
		return emptyObject, nil
	}
	return string(data), nil
}

func scanSampleAnalysis(row scanner) (core.SampleAnalysis, error) {
	var (
		analysis           core.SampleAnalysis
		analyzedModifiedAt sql.NullString
		metricsJSON        string
		attributesJSON     string
		createdAt          sql.NullString
		updatedAt          sql.NullString
	)

	err := row.Scan(
		&analysis.SampleID,
		&analysis.DurationSeconds,
		&analysis.SampleRate,
		&analysis.Channels,
		&analysis.BitDepth,
		&analysis.BPM,
		&analysis.BPMConfidence,
		&analysis.KeyTonic,
		&analysis.KeyMode,
		&analysis.KeyConfidence,
		&analysis.Chord,
		&analysis.ChordConfidence,
		&analysis.PitchHz,
		&analysis.MidiNote,
		&analysis.CentsOffset,
		&analysis.PitchConfidence,
		&analysis.LoopClass,
		&analysis.LoopConfidence,
		&analysis.LUFSIntegrated,
		&analysis.TruePeakDBTP,
		&analyzedModifiedAt,
		&metricsJSON,
		&attributesJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return core.SampleAnalysis{}, err
	}

	analysis.AnalyzedModifiedAt = parseTime(analyzedModifiedAt)
	if err := json.Unmarshal([]byte(metricsJSON), &analysis.Metrics); err != nil {
		return core.SampleAnalysis{}, fmt.Errorf("unmarshal analysis metrics: %w", err)
	}
	if err := json.Unmarshal([]byte(attributesJSON), &analysis.Attributes); err != nil {
		return core.SampleAnalysis{}, fmt.Errorf("unmarshal analysis attributes: %w", err)
	}
	analysis.CreatedAt = parseTime(createdAt)
	analysis.UpdatedAt = parseTime(updatedAt)

	return analysis, nil
}

func scanSampleProjection(row scanner) (core.SampleProjection, error) {
	var (
		projection core.SampleProjection
		createdAt  sql.NullString
		updatedAt  sql.NullString
	)

	err := row.Scan(
		&projection.SampleID,
		&projection.Method,
		&projection.Namespace,
		&projection.X,
		&projection.Y,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return core.SampleProjection{}, err
	}

	projection.CreatedAt = parseTime(createdAt)
	projection.UpdatedAt = parseTime(updatedAt)

	return projection, nil
}

func scanPerceptualHash(row scanner) (core.PerceptualHash, error) {
	var (
		hash      core.PerceptualHash
		createdAt sql.NullString
	)

	err := row.Scan(
		&hash.SampleID,
		&hash.Algorithm,
		&hash.Hash,
		&hash.HashBits,
		&createdAt,
	)
	if err != nil {
		return core.PerceptualHash{}, err
	}

	hash.CreatedAt = parseTime(createdAt)

	return hash, nil
}
