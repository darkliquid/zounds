package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/darkliquid/zounds/pkg/core"
)

type ClusterMember struct {
	ClusterID int64
	SampleID  int64
	Score     float64
}

func (r *Repository) ListFeatureVectors(ctx context.Context, namespace string) ([]core.FeatureVector, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, sample_id, namespace, version, dimensions, values_json, created_at
		FROM feature_vectors
		WHERE namespace = ?
		ORDER BY sample_id, created_at;
	`, namespace)
	if err != nil {
		return nil, fmt.Errorf("list feature vectors for namespace %q: %w", namespace, err)
	}
	defer rows.Close()

	var vectors []core.FeatureVector
	for rows.Next() {
		var (
			vector     core.FeatureVector
			valuesJSON string
			createdAt  sql.NullString
		)
		if err := rows.Scan(&vector.ID, &vector.SampleID, &vector.Namespace, &vector.Version, &vector.Dimensions, &valuesJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scan feature vector: %w", err)
		}
		if err := json.Unmarshal([]byte(valuesJSON), &vector.Values); err != nil {
			return nil, fmt.Errorf("unmarshal feature vector values: %w", err)
		}
		vector.CreatedAt = parseTime(createdAt)
		vectors = append(vectors, vector)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feature vectors: %w", err)
	}
	return vectors, nil
}

func (r *Repository) DeleteClustersByMethod(ctx context.Context, method string) error {
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM clusters
		WHERE method = ?;
	`, method); err != nil {
		return fmt.Errorf("delete clusters by method %q: %w", method, err)
	}
	return nil
}

func (r *Repository) InsertCluster(ctx context.Context, cluster core.Cluster) (int64, error) {
	paramsJSON, err := json.Marshal(cluster.Parameters)
	if err != nil {
		return 0, fmt.Errorf("marshal cluster params: %w", err)
	}

	row := r.db.QueryRowContext(ctx, `
		INSERT INTO clusters (method, label, params_json)
		VALUES (?, ?, ?)
		RETURNING id;
	`, cluster.Method, cluster.Label, string(paramsJSON))

	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, fmt.Errorf("insert cluster %q: %w", cluster.Label, err)
	}
	return id, nil
}

func (r *Repository) InsertClusterMember(ctx context.Context, member ClusterMember) error {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO cluster_members (cluster_id, sample_id, score)
		VALUES (?, ?, ?)
		ON CONFLICT(cluster_id, sample_id) DO UPDATE SET score = excluded.score;
	`, member.ClusterID, member.SampleID, member.Score); err != nil {
		return fmt.Errorf("insert cluster member cluster=%d sample=%d: %w", member.ClusterID, member.SampleID, err)
	}
	return nil
}

func (r *Repository) ListClustersByMethod(ctx context.Context, method string) ([]core.Cluster, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			c.id,
			c.method,
			c.label,
			c.params_json,
			c.created_at,
			COUNT(cm.sample_id) AS size
		FROM clusters c
		LEFT JOIN cluster_members cm ON cm.cluster_id = c.id
		WHERE c.method = ?
		GROUP BY c.id, c.method, c.label, c.params_json, c.created_at
		ORDER BY c.id;
	`, method)
	if err != nil {
		return nil, fmt.Errorf("list clusters by method %q: %w", method, err)
	}
	defer rows.Close()

	var clusters []core.Cluster
	for rows.Next() {
		var (
			cluster    core.Cluster
			paramsJSON string
			createdAt  sql.NullString
		)
		if err := rows.Scan(&cluster.ID, &cluster.Method, &cluster.Label, &paramsJSON, &createdAt, &cluster.Size); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		if err := json.Unmarshal([]byte(paramsJSON), &cluster.Parameters); err != nil {
			return nil, fmt.Errorf("unmarshal cluster params: %w", err)
		}
		cluster.CreatedAt = parseTime(createdAt)
		clusters = append(clusters, cluster)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate clusters: %w", err)
	}
	return clusters, nil
}

func (r *Repository) ListClusterMembers(ctx context.Context, clusterID int64) ([]ClusterMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT cluster_id, sample_id, score
		FROM cluster_members
		WHERE cluster_id = ?
		ORDER BY sample_id;
	`, clusterID)
	if err != nil {
		return nil, fmt.Errorf("list cluster members for cluster %d: %w", clusterID, err)
	}
	defer rows.Close()

	var members []ClusterMember
	for rows.Next() {
		var member ClusterMember
		if err := rows.Scan(&member.ClusterID, &member.SampleID, &member.Score); err != nil {
			return nil, fmt.Errorf("scan cluster member: %w", err)
		}
		members = append(members, member)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cluster members: %w", err)
	}
	return members, nil
}
