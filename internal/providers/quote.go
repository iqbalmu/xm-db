package providers

import "strings"

// QuoteIdentMySQL escapes a MySQL identifier and wraps it in backticks.
func QuoteIdentMySQL(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// QuoteLiteralMySQL escapes a string for use inside single quotes in a MySQL
// statement. Backslash escaping matches the default sql_mode.
func QuoteLiteralMySQL(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "'", `\'`)
	return "'" + r.Replace(s) + "'"
}

// QuoteIdentPG escapes a PostgreSQL identifier and wraps it in double quotes.
func QuoteIdentPG(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// QuoteLiteralPG escapes a string for use inside a standard SQL string literal.
func QuoteLiteralPG(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// SplitTablePG splits an optionally schema-qualified PostgreSQL table name
// ("schema.table" or "table") into schema (defaulting to "public") and table.
func SplitTablePG(name string) (schema, table string) {
	if s, t, ok := strings.Cut(name, "."); ok && s != "" && t != "" {
		return s, t
	}
	return "public", name
}

// QuoteTablePG quotes an optionally schema-qualified PostgreSQL table name.
func QuoteTablePG(name string) string {
	if s, t, ok := strings.Cut(name, "."); ok && s != "" && t != "" {
		return QuoteIdentPG(s) + "." + QuoteIdentPG(t)
	}
	return QuoteIdentPG(name)
}
