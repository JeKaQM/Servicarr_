package database

import (
	"fmt"
	"status/app/internal/buildinfo"
	"strings"
	"time"
)

// SoftwareDeployment records when a particular build was observed running.
type SoftwareDeployment struct {
	Version        string `json:"version"`
	Commit         string `json:"commit"`
	BuildTime      string `json:"build_time,omitempty"`
	FirstStartedAt string `json:"first_started_at"`
	LastStartedAt  string `json:"last_started_at"`
	StartupCount   int    `json:"startup_count"`
}

// RecordSoftwareDeployment stores current build metadata and preserves a small
// restart history for each version/commit pair.
func RecordSoftwareDeployment(info buildinfo.Info) error {
	version := strings.TrimSpace(info.Version)
	if version == "" {
		version = "dev"
	}
	commit := strings.TrimSpace(info.Commit)
	if commit == "" {
		commit = "unknown"
	}
	startedAt := strings.TrimSpace(info.StartedAt)
	if _, err := time.Parse(time.RFC3339, startedAt); err != nil {
		startedAt = time.Now().UTC().Format(time.RFC3339)
	}
	buildTime := strings.TrimSpace(info.BuildTime)

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO software_deployments
		(version, commit_sha, build_time, first_started_at, last_started_at, startup_count)
		VALUES (?, ?, ?, ?, ?, 1)
		ON CONFLICT(version, commit_sha) DO UPDATE SET
			build_time = CASE WHEN excluded.build_time <> '' THEN excluded.build_time ELSE software_deployments.build_time END,
			last_started_at = excluded.last_started_at,
			startup_count = software_deployments.startup_count + 1`,
		version, commit, buildTime, startedAt, startedAt)
	if err != nil {
		return err
	}

	metadata := map[string]string{
		"software_version":         version,
		"software_commit":          commit,
		"software_build_time":      buildTime,
		"software_last_started_at": startedAt,
	}
	for key, value := range metadata {
		if _, err := tx.Exec(`INSERT INTO app_metadata (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetSoftwareDeployments returns the most recently started builds first.
func GetSoftwareDeployments(limit int) ([]SoftwareDeployment, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := DB.Query(`SELECT version, commit_sha, COALESCE(build_time, ''),
		first_started_at, last_started_at, startup_count
		FROM software_deployments
		ORDER BY last_started_at DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deployments := make([]SoftwareDeployment, 0)
	for rows.Next() {
		var deployment SoftwareDeployment
		if err := rows.Scan(
			&deployment.Version,
			&deployment.Commit,
			&deployment.BuildTime,
			&deployment.FirstStartedAt,
			&deployment.LastStartedAt,
			&deployment.StartupCount,
		); err != nil {
			return nil, err
		}
		deployments = append(deployments, deployment)
	}
	return deployments, rows.Err()
}

// SQLiteVersion returns the database engine version for the system panel.
func SQLiteVersion() (string, error) {
	var version string
	if err := DB.QueryRow(`SELECT sqlite_version()`).Scan(&version); err != nil {
		return "", fmt.Errorf("query SQLite version: %w", err)
	}
	return version, nil
}
