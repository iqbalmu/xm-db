package providers

import "testing"

func TestQuoteIdentMySQL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"users", "`users`"},
		{"my`db", "`my``db`"},
		{"", "``"},
	}
	for _, c := range cases {
		if got := QuoteIdentMySQL(c.in); got != c.want {
			t.Errorf("QuoteIdentMySQL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestQuoteLiteralMySQL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "'plain'"},
		{"it's", `'it\'s'`},
		{`back\slash`, `'back\\slash'`},
		{`' \`, `'\' \\'`},
	}
	for _, c := range cases {
		if got := QuoteLiteralMySQL(c.in); got != c.want {
			t.Errorf("QuoteLiteralMySQL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestQuoteIdentPG(t *testing.T) {
	cases := []struct{ in, want string }{
		{"users", `"users"`},
		{`my"db`, `"my""db"`},
	}
	for _, c := range cases {
		if got := QuoteIdentPG(c.in); got != c.want {
			t.Errorf("QuoteIdentPG(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestQuoteLiteralPG(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "'plain'"},
		{"it's", "'it''s'"},
		{`back\slash stays`, `'back\slash stays'`},
	}
	for _, c := range cases {
		if got := QuoteLiteralPG(c.in); got != c.want {
			t.Errorf("QuoteLiteralPG(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
