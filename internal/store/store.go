package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a skill is not found.
var ErrNotFound = errors.New("not found")

// Store provides SQLite-backed storage for the skill catalog index.
type Store struct {
	db *sql.DB
}

// Skill represents a single skill version indexed from the OCI registry.
type Skill struct {
	ID            int64  `json:"-"`
	Repository    string `json:"repository"`
	Tag           string `json:"tag"`
	Digest        string `json:"digest"`
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Version       string `json:"version"`
	Status        string `json:"status"`
	DisplayName   string `json:"display_name"`
	Description   string `json:"description"`
	Authors       string `json:"authors"`
	License       string `json:"license"`
	TagsJSON      string `json:"tags_json"`
	Compatibility string `json:"compatibility"`
	WordCount     int    `json:"word_count"`
	Created       string `json:"created"`
	SyncedAt      string `json:"synced_at"`
}

// Collection represents a collection of skills.
type Collection struct {
	ID          int64  `json:"-"`
	Repository  string `json:"repository"`
	Tag         string `json:"tag"`
	Digest      string `json:"digest"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	SkillsJSON  string `json:"skills_json"`
	Created     string `json:"created"`
	SyncedAt    string `json:"synced_at"`
}

// ListFilter controls which skills are returned by ListSkills.
type ListFilter struct {
	Query         string
	Tags          []string
	Status        string
	Namespace     string
	Compatibility string
	Page          int
	PerPage       int
}

// New opens (or creates) a SQLite database at dsn and ensures the schema exists.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	s := &Store{db: db}
	if err := s.createSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) createSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS skills (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			repository    TEXT NOT NULL,
			tag           TEXT NOT NULL,
			digest        TEXT NOT NULL,
			name          TEXT,
			namespace     TEXT,
			version       TEXT,
			status        TEXT,
			display_name  TEXT,
			description   TEXT,
			authors       TEXT,
			license       TEXT,
			tags_json     TEXT,
			compatibility TEXT,
			word_count    INTEGER DEFAULT 0,
			created       TEXT,
			synced_at     TEXT NOT NULL,
			UNIQUE(repository, tag)
		);
		CREATE INDEX IF NOT EXISTS idx_skills_namespace ON skills(namespace);
		CREATE INDEX IF NOT EXISTS idx_skills_status ON skills(status);
		CREATE INDEX IF NOT EXISTS idx_skills_name ON skills(name);
		CREATE TABLE IF NOT EXISTS collections (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			repository  TEXT NOT NULL,
			tag         TEXT NOT NULL,
			digest      TEXT NOT NULL,
			name        TEXT NOT NULL,
			version     TEXT,
			description TEXT,
			skills_json TEXT NOT NULL,
			created     TEXT,
			synced_at   TEXT NOT NULL,
			UNIQUE(repository, tag)
		);
		CREATE INDEX IF NOT EXISTS idx_collections_name ON collections(name);
	`)
	return err
}

// UpsertSkill inserts a skill or updates it if the (repository, tag) pair already exists.
func (s *Store) UpsertSkill(sk Skill) error {
	sk.SyncedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO skills (repository, tag, digest, name, namespace, version,
			status, display_name, description, authors, license, tags_json,
			compatibility, word_count, created, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository, tag) DO UPDATE SET
			digest=excluded.digest, name=excluded.name, namespace=excluded.namespace,
			version=excluded.version, status=excluded.status,
			display_name=excluded.display_name, description=excluded.description,
			authors=excluded.authors, license=excluded.license,
			tags_json=excluded.tags_json, compatibility=excluded.compatibility,
			word_count=excluded.word_count, created=excluded.created,
			synced_at=excluded.synced_at
	`, sk.Repository, sk.Tag, sk.Digest, sk.Name, sk.Namespace, sk.Version,
		sk.Status, sk.DisplayName, sk.Description, sk.Authors, sk.License,
		sk.TagsJSON, sk.Compatibility, sk.WordCount, sk.Created, sk.SyncedAt)
	return err
}

// ListSkills returns skills matching the given filter criteria.
func (s *Store) ListSkills(f ListFilter) ([]Skill, error) {
	where, args := buildFilterClause(f)
	query := "SELECT id, repository, tag, digest, name, namespace, version, status, display_name, description, authors, license, tags_json, compatibility, word_count, created, synced_at FROM skills" + where
	query += " ORDER BY namespace, name, created DESC"

	if f.PerPage > 0 {
		offset := 0
		if f.Page > 1 {
			offset = (f.Page - 1) * f.PerPage
		}
		query += " LIMIT ? OFFSET ?"
		args = append(args, f.PerPage, offset)
	}

	return s.querySkills(query, args...)
}

// GetSkill returns the latest version of a skill by namespace and name.
func (s *Store) GetSkill(namespace, name string) (*Skill, error) {
	skills, err := s.querySkills(
		"SELECT id, repository, tag, digest, name, namespace, version, status, display_name, description, authors, license, tags_json, compatibility, word_count, created, synced_at FROM skills WHERE namespace = ? AND name = ? ORDER BY created DESC LIMIT 1",
		namespace, name,
	)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, ErrNotFound
	}
	return &skills[0], nil
}

// GetVersions returns all versions of a skill by namespace and name.
func (s *Store) GetVersions(namespace, name string) ([]Skill, error) {
	return s.querySkills(
		"SELECT id, repository, tag, digest, name, namespace, version, status, display_name, description, authors, license, tags_json, compatibility, word_count, created, synced_at FROM skills WHERE namespace = ? AND name = ? ORDER BY created DESC",
		namespace, name,
	)
}

// CountSkills returns the count of skills matching the given filter.
func (s *Store) CountSkills(f ListFilter) (int, error) {
	where, args := buildFilterClause(f)
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM skills"+where, args...).Scan(&count)
	return count, err
}

func buildFilterClause(f ListFilter) (string, []any) {
	clause := " WHERE 1=1"
	var args []any

	if f.Status != "" {
		clause += " AND status = ?"
		args = append(args, f.Status)
	}
	if f.Namespace != "" {
		clause += " AND namespace = ?"
		args = append(args, f.Namespace)
	}
	if f.Compatibility != "" {
		clause += " AND compatibility = ?"
		args = append(args, f.Compatibility)
	}
	if f.Query != "" {
		clause += " AND (name LIKE ? OR display_name LIKE ? OR description LIKE ?)"
		q := "%" + f.Query + "%"
		args = append(args, q, q, q)
	}
	for _, tag := range f.Tags {
		clause += " AND tags_json LIKE ?"
		args = append(args, `%"`+tag+`"%`)
	}

	return clause, args
}

// DeleteStale removes skills that were last synced before the given time.
func (s *Store) DeleteStale(before time.Time) (int64, error) {
	result, err := s.db.Exec("DELETE FROM skills WHERE synced_at < ?",
		before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) querySkills(query string, args ...any) ([]Skill, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var skills []Skill
	for rows.Next() {
		var sk Skill
		if err := rows.Scan(&sk.ID, &sk.Repository, &sk.Tag, &sk.Digest,
			&sk.Name, &sk.Namespace, &sk.Version, &sk.Status,
			&sk.DisplayName, &sk.Description, &sk.Authors, &sk.License,
			&sk.TagsJSON, &sk.Compatibility, &sk.WordCount, &sk.Created,
			&sk.SyncedAt); err != nil {
			return nil, err
		}
		skills = append(skills, sk)
	}
	return skills, rows.Err()
}

// UpsertCollection inserts a collection or updates it if the (repository, tag) pair already exists.
func (s *Store) UpsertCollection(col Collection) error {
	col.SyncedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO collections (repository, tag, digest, name, version,
			description, skills_json, created, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository, tag) DO UPDATE SET
			digest=excluded.digest, name=excluded.name,
			version=excluded.version, description=excluded.description,
			skills_json=excluded.skills_json, created=excluded.created,
			synced_at=excluded.synced_at
	`, col.Repository, col.Tag, col.Digest, col.Name, col.Version,
		col.Description, col.SkillsJSON, col.Created, col.SyncedAt)
	return err
}

// ListCollections returns all collections ordered by name.
func (s *Store) ListCollections() ([]Collection, error) {
	rows, err := s.db.Query(
		"SELECT id, repository, tag, digest, name, version, description, skills_json, created, synced_at FROM collections ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var collections []Collection
	for rows.Next() {
		var col Collection
		if err := rows.Scan(&col.ID, &col.Repository, &col.Tag, &col.Digest,
			&col.Name, &col.Version, &col.Description, &col.SkillsJSON,
			&col.Created, &col.SyncedAt); err != nil {
			return nil, err
		}
		collections = append(collections, col)
	}
	return collections, rows.Err()
}

// GetCollection returns a collection by name.
func (s *Store) GetCollection(name string) (*Collection, error) {
	var col Collection
	err := s.db.QueryRow(
		"SELECT id, repository, tag, digest, name, version, description, skills_json, created, synced_at FROM collections WHERE name = ? ORDER BY synced_at DESC, id DESC LIMIT 1",
		name,
	).Scan(&col.ID, &col.Repository, &col.Tag, &col.Digest,
		&col.Name, &col.Version, &col.Description, &col.SkillsJSON,
		&col.Created, &col.SyncedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &col, nil
}
