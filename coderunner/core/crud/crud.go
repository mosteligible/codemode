package crud

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/mosteligible/mcp-codemode/coderunner/config"
)

const (
	defaultDatabasePort = "5432"
	defaultSSLMode      = "disable"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Connect(ctx context.Context, conf *config.Config) (*sql.DB, error) {
	connectionURL, err := connectionURL(conf)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("postgres", connectionURL)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func ReadRows(ctx context.Context, conf *config.Config, table string, query string, args ...any) ([]map[string]any, error) {
	db, err := Connect(ctx, conf)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	return ReadRowsWithDB(ctx, db, table, query, args...)
}

func DeleteRows(ctx context.Context, conf *config.Config, table string, query string, args ...any) (int64, error) {
	db, err := Connect(ctx, conf)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	return DeleteRowsWithDB(ctx, db, table, query, args...)
}

func ReadRowsWithDB(ctx context.Context, db *sql.DB, table string, query string, args ...any) ([]map[string]any, error) {
	if db == nil {
		return nil, errors.New("missing database connection")
	}

	statement, err := buildReadStatement(table, query)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRows(rows)
}

func DeleteRowsWithDB(ctx context.Context, db *sql.DB, table string, query string, args ...any) (int64, error) {
	if db == nil {
		return 0, errors.New("missing database connection")
	}

	statement, err := buildDeleteStatement(table, query)
	if err != nil {
		return 0, err
	}

	result, err := db.ExecContext(ctx, statement, args...)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func connectionURL(conf *config.Config) (string, error) {
	if conf == nil {
		return "", errors.New("missing database config")
	}
	if strings.TrimSpace(conf.DatabaseHost) == "" {
		return "", errors.New("missing database host")
	}
	if strings.TrimSpace(conf.DatabaseName) == "" {
		return "", errors.New("missing database name")
	}

	port := strings.TrimSpace(conf.DatabasePort)
	if port == "" {
		port = defaultDatabasePort
	}

	dbURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(conf.DatabaseUser, conf.DatabasePassword),
		Host:   net.JoinHostPort(conf.DatabaseHost, port),
		Path:   conf.DatabaseName,
	}
	values := dbURL.Query()
	values.Set("sslmode", defaultSSLMode)
	dbURL.RawQuery = values.Encode()

	return dbURL.String(), nil
}

func buildReadStatement(table string, query string) (string, error) {
	tableIdentifier, err := quoteQualifiedIdentifier(table)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(fmt.Sprintf("SELECT * FROM %s %s", tableIdentifier, whereClause(query))), nil
}

func buildDeleteStatement(table string, query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", errors.New("delete query must not be empty")
	}

	tableIdentifier, err := quoteQualifiedIdentifier(table)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("DELETE FROM %s %s", tableIdentifier, whereClause(query)), nil
}

func quoteQualifiedIdentifier(identifier string) (string, error) {
	parts := strings.Split(strings.TrimSpace(identifier), ".")
	if len(parts) == 0 || len(parts) > 2 {
		return "", fmt.Errorf("invalid table name %q", identifier)
	}

	quotedParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if !identifierPattern.MatchString(part) {
			return "", fmt.Errorf("invalid table name %q", identifier)
		}
		quotedParts = append(quotedParts, pq.QuoteIdentifier(part))
	}

	return strings.Join(quotedParts, "."), nil
}

func whereClause(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(query), "where ") {
		return query
	}
	return "WHERE " + query
}

func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	results := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		valuePointers := make([]any, len(columns))
		for i := range values {
			valuePointers[i] = &values[i]
		}

		if err := rows.Scan(valuePointers...); err != nil {
			return nil, err
		}

		row := make(map[string]any, len(columns))
		for i, column := range columns {
			value := values[i]
			if bytes, ok := value.([]byte); ok {
				value = string(bytes)
			}
			row[column] = value
		}
		results = append(results, row)
	}

	return results, rows.Err()
}
