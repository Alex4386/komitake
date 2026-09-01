package main

import (
	"fmt"
	"strings"
)

// table renders fixed-width columns. The previous CLI emitted raw tab
// characters, which align inconsistently across terminals and break entirely
// once a value is wider than one tab stop.
type table struct {
	headers []string
	rows    [][]string
}

func newTable(headers ...string) *table {
	return &table{headers: headers}
}

func (t *table) Add(cells ...string) {
	row := make([]string, len(t.headers))
	for i := range row {
		if i < len(cells) {
			row[i] = cells[i]
			continue
		}
		row[i] = "-"
	}
	t.rows = append(t.rows, row)
}

// widths measures the natural width of every column, header included.
func (t *table) widths() []int {
	w := make([]int, len(t.headers))
	for i, h := range t.headers {
		w[i] = len([]rune(h))
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if n := len([]rune(cell)); n > w[i] {
				w[i] = n
			}
		}
	}
	return w
}

// Render writes the table. When the output is not a terminal, columns are
// tab-separated and headers omitted so awk/cut pipelines stay simple.
func (t *table) Render(u *ui) {
	if !u.tty {
		for _, row := range t.rows {
			u.Println(strings.Join(row, "\t"))
		}
		return
	}

	widths := t.widths()
	var b strings.Builder
	for i, h := range t.headers {
		b.WriteString(pad(strings.ToUpper(h), widths[i], i == len(t.headers)-1))
		if i < len(t.headers)-1 {
			b.WriteString("  ")
		}
	}
	u.Printf("%s\n", u.paint(u.c.dim, b.String()))

	for _, row := range t.rows {
		line := make([]string, len(row))
		for i, cell := range row {
			line[i] = pad(cell, widths[i], i == len(row)-1)
		}
		u.Println(strings.Join(line, "  "))
	}
}

// pad right-aligns to width, skipping padding on the final column to avoid
// trailing whitespace.
func pad(s string, width int, last bool) string {
	if last {
		return s
	}
	return fmt.Sprintf("%-*s", width, s)
}
