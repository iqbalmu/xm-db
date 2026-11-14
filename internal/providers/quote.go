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
