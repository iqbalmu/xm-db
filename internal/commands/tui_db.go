package commands

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/iqbalmu/xm-db/pkg/utils"
)

type tableDetail struct {
	tableName   string
	schemaCols  []string
	schemaRows  [][]string
	previewCols []string
	previewRows [][]string
}

type tableColumnInfo struct {
	name     string
	dataType string
	nullable bool
	key      string // "PK", "FK", "PK,FK", "UNI", or ""
	defVal   string
}

type tableRelation struct {
	fromTable string
	fromCol   string
	toTable   string
	toCol     string
}

type tableOverview struct {
	name    string
	columns []tableColumnInfo
}

type tableSchemaGraph struct {
	tables    []tableOverview
	byName    map[string]*tableOverview
	relations []tableRelation
}

type sqlQueryResult struct {
	query    string
	cols     []string
	rows     [][]string
	affected int64
	isSelect bool
}

func openDBConn(dbState *state.Database) (providers.Provider, *sql.DB, error) {
	prov, err := providers.For(dbState.Provider)
	if err != nil {
		return nil, nil, err
	}
	conn, err := prov.Connect(&providers.DBConfig{
		Host:         "127.0.0.1",
		Port:         dbState.Port,
		RootPassword: dbState.RootPassword,
		DatabaseName: dbState.DatabaseName,
	})
	if err != nil {
		return nil, nil, err
	}
	return prov, conn, nil
}

func fetchTableDetail(ctx context.Context, dbState *state.Database, tableName string) (*tableDetail, error) {
	_, conn, err := openDBConn(dbState)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	detail := &tableDetail{tableName: tableName}

	var schemaQ string
	var previewQ string
	if dbState.Provider == utils.ProviderMySQL {
		quoted := providers.QuoteIdentMySQL(tableName)
		schemaQ = fmt.Sprintf("DESCRIBE %s", quoted)
		previewQ = fmt.Sprintf("SELECT * FROM %s LIMIT 25", quoted)
	} else {
		schema, tbl := providers.SplitTablePG(tableName)
		schemaQ = fmt.Sprintf(`
			SELECT column_name, data_type, is_nullable, COALESCE(column_default, 'NULL')
			FROM information_schema.columns
			WHERE table_schema = %s AND table_name = %s
			ORDER BY ordinal_position`, providers.QuoteLiteralPG(schema), providers.QuoteLiteralPG(tbl))
		previewQ = fmt.Sprintf("SELECT * FROM %s LIMIT 25", providers.QuoteTablePG(tableName))
	}

	sRows, err := conn.QueryContext(ctx, schemaQ)
	if err != nil {
		return nil, fmt.Errorf("describing table: %w", err)
	}
	detail.schemaCols, detail.schemaRows, err = scanAllRows(sRows, 200)
	sRows.Close()
	if err != nil {
		return nil, err
	}

	pRows, err := conn.QueryContext(ctx, previewQ)
	if err == nil {
		detail.previewCols, detail.previewRows, _ = scanAllRows(pRows, 25)
		pRows.Close()
	}

	return detail, nil
}

func executeQuickSQL(ctx context.Context, dbState *state.Database, query string) (*sqlQueryResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("empty query")
	}

	_, conn, err := openDBConn(dbState)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	upper := strings.ToUpper(q)
	isSelect := strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "DESCRIBE") ||
		strings.HasPrefix(upper, "DESC ") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "WITH")

	res := &sqlQueryResult{
		query:    q,
		isSelect: isSelect,
	}

	if isSelect {
		rows, err := conn.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		res.cols, res.rows, err = scanAllRows(rows, 200)
		if err != nil {
			return nil, err
		}
		return res, nil
	}

	execRes, err := conn.ExecContext(ctx, q)
	if err != nil {
		return nil, err
	}
	affected, _ := execRes.RowsAffected()
	res.affected = affected
	return res, nil
}

func scanAllRows(rows *sql.Rows, maxRows int) ([]string, [][]string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out [][]string
	for rows.Next() {
		if maxRows > 0 && len(out) >= maxRows {
			break
		}
		vals := make([]sql.RawBytes, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			if v == nil {
				row[i] = "NULL"
			} else {
				s := strings.ReplaceAll(string(v), "\n", " ")
				s = strings.ReplaceAll(s, "\r", "")
				row[i] = s
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return cols, out, nil
}

func formatCompactTable(cols []string, rows [][]string, maxWidth int) string {
	if len(cols) == 0 {
		return styleMutedText.Render("(no columns)")
	}
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = lipgloss.Width(c)
	}
	for _, r := range rows {
		for i := range cols {
			if i < len(r) {
				if w := lipgloss.Width(r[i]); w > widths[i] {
					widths[i] = w
				}
			}
		}
	}
	const maxColW = 28
	for i := range widths {
		if widths[i] > maxColW {
			widths[i] = maxColW
		}
		if widths[i] < 3 {
			widths[i] = 3
		}
	}

	renderRow := func(cells []string) string {
		parts := make([]string, len(cols))
		for i := range cols {
			val := ""
			if i < len(cells) {
				val = cells[i]
			}
			if lipgloss.Width(val) > widths[i] {
				val = ansi.Truncate(val, widths[i]-1, "…")
			}
			pad := widths[i] - lipgloss.Width(val)
			if pad > 0 {
				val += strings.Repeat(" ", pad)
			}
			parts[i] = val
		}
		line := strings.Join(parts, " │ ")
		if maxWidth > 0 && lipgloss.Width(line) > maxWidth {
			line = ansi.Truncate(line, maxWidth-1, "…")
		}
		return line
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(renderRow(cols)) + "\n")

	sepParts := make([]string, len(cols))
	for i, w := range widths {
		sepParts[i] = strings.Repeat("─", w)
	}
	sep := strings.Join(sepParts, "─┼─")
	if maxWidth > 0 && lipgloss.Width(sep) > maxWidth {
		sep = ansi.Truncate(sep, maxWidth, "")
	}
	b.WriteString(styleMutedText.Render(sep) + "\n")

	if len(rows) == 0 {
		b.WriteString(styleMutedText.Render("(0 rows)") + "\n")
		return b.String()
	}
	for _, r := range rows {
		b.WriteString(renderRow(r) + "\n")
	}
	return b.String()
}

func fetchTableSchemaGraph(ctx context.Context, conn *sql.DB, provider, dbName string, tables []string) *tableSchemaGraph {
	graph := &tableSchemaGraph{
		tables: make([]tableOverview, len(tables)),
		byName: make(map[string]*tableOverview, len(tables)),
	}
	for i, t := range tables {
		graph.tables[i] = tableOverview{name: t}
		graph.byName[t] = &graph.tables[i]
	}
	if len(tables) == 0 || conn == nil {
		return graph
	}

	if provider == utils.ProviderMySQL {
		colRows, err := conn.QueryContext(ctx, `
			SELECT table_name, column_name, column_type, is_nullable, column_key, COALESCE(column_default, '')
			FROM information_schema.columns
			WHERE table_schema = ?
			ORDER BY table_name, ordinal_position`, dbName)
		if err == nil {
			defer colRows.Close()
			for colRows.Next() {
				var tbl, col, dtype, nullable, colKey, defVal string
				if err := colRows.Scan(&tbl, &col, &dtype, &nullable, &colKey, &defVal); err != nil {
					continue
				}
				t := graph.byName[tbl]
				if t == nil {
					continue
				}
				key := ""
				switch colKey {
				case "PRI":
					key = "PK"
				case "UNI":
					key = "UNI"
				case "MUL":
					key = "IDX"
				}
				t.columns = append(t.columns, tableColumnInfo{
					name:     col,
					dataType: dtype,
					nullable: strings.EqualFold(nullable, "YES"),
					key:      key,
					defVal:   defVal,
				})
			}
		}

		fkRows, err := conn.QueryContext(ctx, `
			SELECT table_name, column_name, referenced_table_name, referenced_column_name
			FROM information_schema.key_column_usage
			WHERE table_schema = ? AND referenced_table_name IS NOT NULL
			ORDER BY table_name, ordinal_position`, dbName)
		if err == nil {
			defer fkRows.Close()
			for fkRows.Next() {
				var fromTbl, fromCol, toTbl, toCol string
				if err := fkRows.Scan(&fromTbl, &fromCol, &toTbl, &toCol); err != nil {
					continue
				}
				graph.relations = append(graph.relations, tableRelation{
					fromTable: fromTbl,
					fromCol:   fromCol,
					toTable:   toTbl,
					toCol:     toCol,
				})
				markColumnFK(graph.byName[fromTbl], fromCol)
			}
		}
		return graph
	}

	// PostgreSQL
	colRows, err := conn.QueryContext(ctx, `
		SELECT
			CASE WHEN c.table_schema = 'public' THEN c.table_name ELSE c.table_schema || '.' || c.table_name END AS tbl,
			c.column_name,
			c.data_type,
			c.is_nullable,
			COALESCE(c.column_default, '')
		FROM information_schema.columns c
		JOIN information_schema.tables t
			ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema NOT IN ('pg_catalog', 'information_schema')
			AND t.table_type = 'BASE TABLE'
		ORDER BY c.table_schema = 'public' DESC, c.table_schema, c.table_name, c.ordinal_position`)
	if err == nil {
		defer colRows.Close()
		for colRows.Next() {
			var tbl, col, dtype, nullable, defVal string
			if err := colRows.Scan(&tbl, &col, &dtype, &nullable, &defVal); err != nil {
				continue
			}
			t := graph.byName[tbl]
			if t == nil {
				continue
			}
			t.columns = append(t.columns, tableColumnInfo{
				name:     col,
				dataType: dtype,
				nullable: strings.EqualFold(nullable, "YES"),
				defVal:   defVal,
			})
		}
	}

	constRows, err := conn.QueryContext(ctx, `
		SELECT
			tc.constraint_type,
			CASE WHEN tc.table_schema = 'public' THEN tc.table_name ELSE tc.table_schema || '.' || tc.table_name END AS from_tbl,
			kcu.column_name AS from_col,
			COALESCE(CASE WHEN ccu.table_schema = 'public' THEN ccu.table_name ELSE ccu.table_schema || '.' || ccu.table_name END, '') AS to_tbl,
			COALESCE(ccu.column_name, '') AS to_col
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		LEFT JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_type = 'FOREIGN KEY' AND tc.constraint_name = ccu.constraint_name AND tc.table_schema = ccu.table_schema
		WHERE tc.table_schema NOT IN ('pg_catalog', 'information_schema')
			AND tc.constraint_type IN ('PRIMARY KEY', 'FOREIGN KEY')
		ORDER BY from_tbl, kcu.ordinal_position`)
	if err == nil {
		defer constRows.Close()
		for constRows.Next() {
			var ctype, fromTbl, fromCol, toTbl, toCol string
			if err := constRows.Scan(&ctype, &fromTbl, &fromCol, &toTbl, &toCol); err != nil {
				continue
			}
			if ctype == "PRIMARY KEY" {
				markColumnPK(graph.byName[fromTbl], fromCol)
			} else if ctype == "FOREIGN KEY" && toTbl != "" {
				graph.relations = append(graph.relations, tableRelation{
					fromTable: fromTbl,
					fromCol:   fromCol,
					toTable:   toTbl,
					toCol:     toCol,
				})
				markColumnFK(graph.byName[fromTbl], fromCol)
			}
		}
	}

	return graph
}

func markColumnPK(t *tableOverview, colName string) {
	if t == nil {
		return
	}
	for i := range t.columns {
		if t.columns[i].name == colName {
			if t.columns[i].key == "FK" {
				t.columns[i].key = "PK,FK"
			} else if t.columns[i].key != "PK,FK" {
				t.columns[i].key = "PK"
			}
		}
	}
}

func markColumnFK(t *tableOverview, colName string) {
	if t == nil {
		return
	}
	for i := range t.columns {
		if t.columns[i].name == colName {
			if t.columns[i].key == "PK" {
				t.columns[i].key = "PK,FK"
			} else if t.columns[i].key != "PK,FK" {
				t.columns[i].key = "FK"
			}
		}
	}
}

func formatTableOverviewList(tables []string, graph *tableSchemaGraph, cursor int, focused bool, showFields bool, maxWidth int) (string, int) {
	var b strings.Builder
	lineNo := 0
	cursorLine := 0

	for i, t := range tables {
		var cols []tableColumnInfo
		if graph != nil {
			if ov := graph.byName[t]; ov != nil {
				cols = ov.columns
			}
		}

		countSuffix := ""
		if len(cols) > 0 {
			countSuffix = fmt.Sprintf(" (%d fields)", len(cols))
		}

		prefix := "  "
		if focused && i == cursor {
			prefix = "▸ "
		}
		if i == cursor {
			cursorLine = lineNo
		}

		header := fmt.Sprintf("%s%d. %s%s", prefix, i+1, t, countSuffix)
		if maxWidth > 4 && lipgloss.Width(header) > maxWidth {
			header = ansi.Truncate(header, maxWidth-1, "…")
		}
		if focused && i == cursor {
			header = styleSelectedRow.Render(header)
		}
		b.WriteString(header + "\n")
		lineNo++

		if showFields && len(cols) > 0 {
			for _, col := range cols {
				badge := "   "
				if col.key != "" {
					badge = fmt.Sprintf("[%-2s]", col.key)
				}
				line := fmt.Sprintf("     • %s %-18s %s", badge, col.name, col.dataType)
				if maxWidth > 4 && lipgloss.Width(line) > maxWidth {
					line = ansi.Truncate(line, maxWidth-1, "…")
				}
				b.WriteString(styleMutedText.Render(line) + "\n")
				lineNo++
			}
		}
	}

	return b.String(), cursorLine
}

func formatTableOverviewDiagram(tables []string, graph *tableSchemaGraph, cursor int, focused bool, showFields bool, maxWidth int) (string, int) {
	if maxWidth < 24 {
		maxWidth = 24
	}

	// Choose box width so 1-3 boxes fit nicely per row.
	gap := 2
	colsPerRow := 1
	if maxWidth >= 92 && !showFields {
		colsPerRow = 3
	} else if maxWidth >= 60 {
		colsPerRow = 2
	}
	if !showFields && maxWidth >= 60 && colsPerRow < 2 {
		colsPerRow = 2
	}
	boxW := (maxWidth - gap*(colsPerRow-1)) / colsPerRow
	if boxW < 22 {
		colsPerRow = 1
		boxW = maxWidth
	}
	if boxW > 44 {
		boxW = 44
	}

	renderCard := func(idx int, tName string) string {
		var cols []tableColumnInfo
		if graph != nil {
			if ov := graph.byName[tName]; ov != nil {
				cols = ov.columns
			}
		}

		selected := focused && idx == cursor
		borderColor := colorMuted
		titleColor := colorAccent
		marker := ""
		if selected {
			borderColor = colorPrimary
			titleColor = colorPrimary
			marker = "▸ "
		}

		innerW := max(12, boxW-4)
		title := marker + tName
		if lipgloss.Width(title) > innerW {
			title = ansi.Truncate(title, innerW-1, "…")
		}

		var body strings.Builder
		body.WriteString(lipgloss.NewStyle().Bold(true).Foreground(titleColor).Render(title))

		if showFields {
			if len(cols) == 0 {
				body.WriteString("\n" + styleMutedText.Render("(no columns)"))
			} else {
				for _, col := range cols {
					keyTag := "  "
					if strings.Contains(col.key, "PK") {
						keyTag = "PK"
					} else if strings.Contains(col.key, "FK") {
						keyTag = "FK"
					}
					nameW := max(6, innerW-13)
					cName := col.name
					if lipgloss.Width(cName) > nameW {
						cName = ansi.Truncate(cName, nameW-1, "…")
					}
					dType := col.dataType
					if lipgloss.Width(dType) > 8 {
						dType = ansi.Truncate(dType, 7, "…")
					}
					rowLine := fmt.Sprintf("%s %-*s %s", keyTag, nameW, cName, dType)
					if lipgloss.Width(rowLine) > innerW {
						rowLine = ansi.Truncate(rowLine, innerW-1, "…")
					}
					body.WriteString("\n" + rowLine)
				}
			}
		} else {
			sub := fmt.Sprintf("(%d fields)", len(cols))
			body.WriteString("\n" + styleMutedText.Render(sub))
		}

		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Width(boxW - 2).
			Padding(0, 1).
			Render(body.String())
		return box
	}

	var out strings.Builder
	lineNo := 0
	cursorLine := 0

	for start := 0; start < len(tables); start += colsPerRow {
		end := min(len(tables), start+colsPerRow)
		cards := make([]string, 0, end-start)
		hasCursor := false
		for i := start; i < end; i++ {
			if i == cursor {
				hasCursor = true
			}
			cards = append(cards, renderCard(i, tables[i]))
		}
		if hasCursor {
			cursorLine = lineNo
		}
		rowStr := lipgloss.JoinHorizontal(lipgloss.Top, joinWithGap(cards, strings.Repeat(" ", gap))...)
		out.WriteString(rowStr + "\n")
		lineNo += strings.Count(rowStr, "\n") + 1
	}

	if graph != nil && len(graph.relations) > 0 {
		out.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("Relationships") + "\n")
		seen := make(map[string]bool)
		for _, rel := range graph.relations {
			var line string
			if showFields {
				line = fmt.Sprintf("  %s.%s ──▶ %s.%s", rel.fromTable, rel.fromCol, rel.toTable, rel.toCol)
			} else {
				line = fmt.Sprintf("  %s ──▶ %s", rel.fromTable, rel.toTable)
			}
			if seen[line] {
				continue
			}
			seen[line] = true
			if lipgloss.Width(line) > maxWidth {
				line = ansi.Truncate(line, maxWidth-1, "…")
			}
			out.WriteString(styleMutedText.Render(line) + "\n")
		}
	}

	return out.String(), cursorLine
}

func joinWithGap(items []string, gap string) []string {
	if len(items) <= 1 {
		return items
	}
	res := make([]string, 0, len(items)*2-1)
	for i, it := range items {
		if i > 0 {
			res = append(res, gap)
		}
		res = append(res, it)
	}
	return res
}

func formatTableDetailDiagram(detail *tableDetail, graph *tableSchemaGraph, showFields bool, maxWidth int) string {
	if detail == nil {
		return ""
	}
	boxW := min(max(32, maxWidth), 76)
	innerW := max(20, boxW-4)

	var cols []tableColumnInfo
	if graph != nil {
		if ov := graph.byName[detail.tableName]; ov != nil {
			cols = ov.columns
		}
	}
	// Fallback to schemaRows if graph wasn't populated for this table.
	if len(cols) == 0 && len(detail.schemaRows) > 0 {
		for _, r := range detail.schemaRows {
			if len(r) >= 2 {
				cols = append(cols, tableColumnInfo{
					name:     r[0],
					dataType: r[1],
					nullable: len(r) > 2 && strings.EqualFold(r[2], "YES"),
				})
			}
		}
	}

	var body strings.Builder
	header := fmt.Sprintf("%s (%d fields)", detail.tableName, len(cols))
	body.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(header))

	if showFields {
		if len(cols) == 0 {
			body.WriteString("\n" + styleMutedText.Render("(no fields)"))
		} else {
			for _, col := range cols {
				tag := "   "
				if col.key != "" {
					tag = fmt.Sprintf("%-3s", col.key)
				}
				nullStr := "NOT NULL"
				if col.nullable {
					nullStr = "NULL"
				}
				line := fmt.Sprintf("%s %-18s %-14s %s", tag, col.name, col.dataType, nullStr)
				if lipgloss.Width(line) > innerW {
					line = ansi.Truncate(line, innerW-1, "…")
				}
				body.WriteString("\n" + line)
			}
		}
	} else {
		body.WriteString("\n" + styleMutedText.Render("(fields hidden — press 'f' to show)"))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Width(boxW - 2).
		Padding(0, 1).
		Render(body.String())

	var out strings.Builder
	out.WriteString(box + "\n")

	if graph != nil {
		var relLines []string
		seen := make(map[string]bool)
		for _, rel := range graph.relations {
			if rel.fromTable != detail.tableName && rel.toTable != detail.tableName {
				continue
			}
			var line string
			if showFields {
				if rel.fromTable == detail.tableName {
					line = fmt.Sprintf("  %s.%s ──▶ %s.%s", rel.fromTable, rel.fromCol, rel.toTable, rel.toCol)
				} else {
					line = fmt.Sprintf("  %s.%s ◀── %s.%s", detail.tableName, rel.toCol, rel.fromTable, rel.fromCol)
				}
			} else {
				if rel.fromTable == detail.tableName {
					line = fmt.Sprintf("  %s ──▶ %s", rel.fromTable, rel.toTable)
				} else {
					line = fmt.Sprintf("  %s ◀── %s", detail.tableName, rel.fromTable)
				}
			}
			if !seen[line] {
				seen[line] = true
				relLines = append(relLines, line)
			}
		}
		if len(relLines) > 0 {
			out.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("Relationships") + "\n")
			for _, l := range relLines {
				out.WriteString(styleMutedText.Render(l) + "\n")
			}
		}
	}

	return out.String()
}
