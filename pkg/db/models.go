package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(database *sql.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) UpsertSample(ctx context.Context, sample core.Sample) (int64, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO samples (
			source_root,
			path,
			relative_path,
			file_name,
			extension,
			format,
			size_bytes,
			modified_at,
			scanned_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			source_root = excluded.source_root,
			relative_path = excluded.relative_path,
			file_name = excluded.file_name,
			extension = excluded.extension,
			format = excluded.format,
			size_bytes = excluded.size_bytes,
			modified_at = excluded.modified_at,
			scanned_at = excluded.scanned_at
		RETURNING id;
	`,
		sample.SourceRoot,
		sample.Path,
		sample.RelativePath,
		sample.FileName,
		sample.Extension,
		string(sample.Format),
		sample.SizeBytes,
		timeToValue(sample.ModifiedAt),
		timeToValue(sample.ScannedAt),
	)

	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert sample %q: %w", sample.Path, err)
	}

	return id, nil
}

func (r *Repository) ListSamples(ctx context.Context) ([]core.Sample, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			id,
			source_root,
			path,
			relative_path,
			file_name,
			extension,
			format,
			size_bytes,
			modified_at,
			scanned_at
		FROM samples
		ORDER BY path;
	`)
	if err != nil {
		return nil, fmt.Errorf("list samples: %w", err)
	}
	defer rows.Close()

	var samples []core.Sample
	for rows.Next() {
		sample, err := scanSample(rows)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate samples: %w", err)
	}

	return samples, nil
}

func (r *Repository) FindSampleByPath(ctx context.Context, path string) (core.Sample, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			id,
			source_root,
			path,
			relative_path,
			file_name,
			extension,
			format,
			size_bytes,
			modified_at,
			scanned_at
		FROM samples
		WHERE path = ?;
	`, path)

	sample, err := scanSample(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return core.Sample{}, err
		}
		return core.Sample{}, fmt.Errorf("find sample by path %q: %w", path, err)
	}

	return sample, nil
}

func (r *Repository) InsertFeatureVector(ctx context.Context, vector core.FeatureVector) (int64, error) {
	valuesJSON, err := json.Marshal(vector.Values)
	if err != nil {
		return 0, fmt.Errorf("marshal feature vector values: %w", err)
	}

	row := r.db.QueryRowContext(ctx, `
		INSERT INTO feature_vectors (
			sample_id,
			namespace,
			version,
			dimensions,
			values_json
		) VALUES (?, ?, ?, ?, ?)
		RETURNING id;
	`, vector.SampleID, vector.Namespace, vector.Version, len(vector.Values), string(valuesJSON))

	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, fmt.Errorf("insert feature vector: %w", err)
	}

	return id, nil
}

func timeToValue(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSample(row scanner) (core.Sample, error) {
	var (
		sample     core.Sample
		format     string
		modifiedAt sql.NullString
		scannedAt  sql.NullString
	)

	err := row.Scan(
		&sample.ID,
		&sample.SourceRoot,
		&sample.Path,
		&sample.RelativePath,
		&sample.FileName,
		&sample.Extension,
		&format,
		&sample.SizeBytes,
		&modifiedAt,
		&scannedAt,
	)
	if err != nil {
		return core.Sample{}, err
	}

	sample.Format = core.AudioFormat(format)
	sample.ModifiedAt = parseTime(modifiedAt)
	sample.ScannedAt = parseTime(scannedAt)

	return sample, nil
}

func parseTime(value sql.NullString) time.Time {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return time.Time{}
	}

	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return time.Time{}
	}

	return parsed
}
