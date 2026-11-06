package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Optimize runs a thorough PRAGMA optimize, checking all tables, and merges
// the b-trees of all FTS5 tables. Run it on startup and periodically,
// e.g. with app.Every.
func Optimize(ctx context.Context, db *sql.DB) error {
	start := time.Now()

	if _, err := db.ExecContext(ctx, `PRAGMA optimize=0x10002`); err != nil {
		return err
	}

	tables, err := listFTS5Tables(ctx, db)
	if err != nil {
		return err
	}

	for _, table := range tables {
		ident := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %[1]s(%[1]s) VALUES('optimize')`, ident)); err != nil {
			return fmt.Errorf("error optimizing FTS table %q: %w", table, err)
		}
	}

	slog.Info(fmt.Sprintf("database optimization finished in %v", time.Since(start)))

	return nil
}

func listFTS5Tables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name FROM sqlite_schema
		WHERE type = 'table'
		AND sql LIKE 'CREATE VIRTUAL TABLE%USING fts5%'
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}

	return tables, rows.Err()
}
