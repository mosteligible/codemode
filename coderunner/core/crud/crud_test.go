package crud

import "testing"

func TestBuildReadStatement(t *testing.T) {
	got, err := buildReadStatement("client_proxies", "client_id = $1")
	if err != nil {
		t.Fatalf("buildReadStatement() returned error: %v", err)
	}

	want := `SELECT * FROM "client_proxies" WHERE client_id = $1`
	if got != want {
		t.Fatalf("buildReadStatement() = %q, want %q", got, want)
	}
}

func TestBuildDeleteStatement(t *testing.T) {
	got, err := buildDeleteStatement("public.client_proxies", "WHERE proxy_id = $1")
	if err != nil {
		t.Fatalf("buildDeleteStatement() returned error: %v", err)
	}

	want := `DELETE FROM "public"."client_proxies" WHERE proxy_id = $1`
	if got != want {
		t.Fatalf("buildDeleteStatement() = %q, want %q", got, want)
	}
}

func TestBuildDeleteStatementRequiresQuery(t *testing.T) {
	_, err := buildDeleteStatement("client_proxies", "")
	if err == nil {
		t.Fatal("buildDeleteStatement() succeeded unexpectedly")
	}
}

func TestQuoteQualifiedIdentifierRejectsInvalidTable(t *testing.T) {
	_, err := quoteQualifiedIdentifier("client_proxies; DROP TABLE users")
	if err == nil {
		t.Fatal("quoteQualifiedIdentifier() succeeded unexpectedly")
	}
}
