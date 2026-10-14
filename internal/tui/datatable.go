package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	dataTableCellPadding = 1
	tablePrefixWidth     = 2 // selection marker and a space
	tableColumnGap       = 3
)

type dataTableColumn struct {
	Title    string
	Width    int // Fixed content width when Flex is zero; compressed only if necessary to fit.
	Flex     int // Relative share of unused width when positive.
	MinWidth int // Minimum content width for a flex column when space permits.
	MaxWidth int // Cap for a flex column; unused width stays empty.
	Right    bool
	Tight    bool // use a one-space gap after this column
}

func (column dataTableColumn) gapAfter() int {
	if column.Tight {
		return 1
	}
	return tableColumnGap
}

// tableCell is one value: an optional status glyph followed by text, each with
// its own style so the selected row can add a background without losing them.
type tableCell struct {
	Text       string
	Style      lipgloss.Style
	Glyph      string
	GlyphStyle lipgloss.Style
}

type dataTable struct {
	Columns     []dataTableColumn
	Rows        [][]tableCell
	SelectedRow int
	Width       int
	Height      int  // total rows including the header; zero means unlimited
	Focused     bool // the selected row is highlighted only in the focused pane
}

// View renders a header and the visible window of rows. It also returns the
// index of the first visible row, for mouse hit testing.
func (table dataTable) View() (string, int) {
	if len(table.Columns) == 0 {
		return "", 0
	}
	count := len(table.Columns)
	available := table.Width - tablePrefixWidth
	for _, column := range table.Columns[:count-1] {
		available -= column.gapAfter()
	}
	// responsiveColumnWidths reserves cell padding that this layout replaces
	// with explicit gaps, so hand it the viewport with that padding added.
	widths := responsiveColumnWidths(table.Columns, available+2*dataTableCellPadding*count)

	header := strings.Repeat(" ", tablePrefixWidth)
	for index, column := range table.Columns {
		if index > 0 {
			header += strings.Repeat(" ", table.Columns[index-1].gapAfter())
		}
		header += mutedStyle.Render(alignCell(column.Title, widths[index], column.Right))
	}
	lines := []string{header}

	budget := len(table.Rows)
	if table.Height > 0 {
		budget = max(1, table.Height-1)
	}
	start := windowStart(len(table.Rows), table.SelectedRow, budget)
	end := min(len(table.Rows), start+budget)
	for index := start; index < end; index++ {
		lines = append(lines, table.renderRow(index, table.Rows[index], widths))
	}
	return strings.Join(lines, "\n"), start
}

func (table dataTable) renderRow(index int, row []tableCell, widths []int) string {
	selected := index == table.SelectedRow
	focused := selected && table.Focused
	segment := func(style lipgloss.Style, value string) string {
		if focused {
			style = style.Background(colorRule)
		}
		return style.Render(value)
	}
	marker := segment(lipgloss.NewStyle(), " ")
	switch {
	case focused:
		marker = segment(activeStyle, "▌")
	case selected:
		marker = dimStyle.Render("▌")
	}
	line := marker + segment(lipgloss.NewStyle(), " ")
	for column := range table.Columns {
		if column > 0 {
			line += segment(lipgloss.NewStyle(), strings.Repeat(" ", table.Columns[column-1].gapAfter()))
		}
		var cell tableCell
		if column < len(row) {
			cell = row[column]
		}
		line += renderTableCell(cell, widths[column], table.Columns[column].Right, column == 0 && focused, segment)
	}
	if gap := table.Width - ansi.StringWidth(line); gap > 0 {
		line += segment(lipgloss.NewStyle(), strings.Repeat(" ", gap))
	}
	return line
}

func renderTableCell(cell tableCell, width int, right, emphasize bool, segment func(lipgloss.Style, string) string) string {
	style := cell.Style
	if emphasize {
		style = activeStyle
	}
	text := ansi.Truncate(cell.Text, max(0, width-ansi.StringWidth(cell.Glyph)-boolInt(cell.Glyph != "")), "…")
	used := ansi.StringWidth(text)
	if cell.Glyph != "" {
		used += ansi.StringWidth(cell.Glyph) + 1
	}
	pad := segment(lipgloss.NewStyle(), strings.Repeat(" ", max(0, width-used)))
	var content string
	if cell.Glyph != "" {
		content = segment(cell.GlyphStyle, cell.Glyph) + segment(lipgloss.NewStyle(), " ")
	}
	content += segment(style, text)
	if right {
		return pad + content
	}
	return content + pad
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func alignCell(value string, width int, right bool) string {
	value = ansi.Truncate(value, width, "")
	pad := strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
	if right {
		return pad + value
	}
	return value + pad
}

// responsiveColumnWidths returns bubbles/table content widths. Its padding is
// added by the table styles, so subtract it before distributing the viewport.
func responsiveColumnWidths(columns []dataTableColumn, viewportWidth int) []int {
	widths := distributeColumnWidths(columns, viewportWidth)
	for index, column := range columns {
		if column.Flex > 0 && column.MaxWidth > 0 && widths[index] > column.MaxWidth {
			widths[index] = column.MaxWidth
		}
	}
	return widths
}

func distributeColumnWidths(columns []dataTableColumn, viewportWidth int) []int {
	widths := make([]int, len(columns))
	flexColumns := make([]int, 0, len(columns))
	fixedColumns := make([]int, 0, len(columns))
	flexWeight := 0

	for index, column := range columns {
		if column.Flex > 0 {
			minimum := max(column.MinWidth, ansi.StringWidth(column.Title))
			if minimum < 1 {
				minimum = 1
			}
			widths[index] = minimum
			flexColumns = append(flexColumns, index)
			flexWeight += column.Flex
			continue
		}

		width := column.Width
		if width <= 0 {
			width = max(1, ansi.StringWidth(column.Title))
		}
		widths[index] = width
		fixedColumns = append(fixedColumns, index)
	}

	if viewportWidth <= 0 {
		return widths
	}
	contentWidth := max(0, viewportWidth-2*dataTableCellPadding*len(columns))
	remaining := contentWidth - widthsTotal(widths)
	if remaining < 0 {
		// Keep requested flex minima and fixed widths where possible; compress
		// fixed columns to their titles, then flex columns, only if the viewport
		// is too small to honor all configured widths.
		fixedMinimums := make([]int, len(columns))
		for _, index := range fixedColumns {
			fixedMinimums[index] = min(widths[index], max(1, ansi.StringWidth(columns[index].Title)))
		}
		remaining = shrinkColumnWidths(widths, fixedColumns, fixedMinimums, remaining)
		flexMinimums := make([]int, len(columns))
		for _, index := range flexColumns {
			flexMinimums[index] = 1
		}
		remaining = shrinkColumnWidths(widths, flexColumns, flexMinimums, remaining)
		if remaining < 0 {
			minimums := make([]int, len(columns))
			for _, index := range fixedColumns {
				minimums[index] = 1
			}
			shrinkColumnWidths(widths, fixedColumns, minimums, remaining)
		}
		return widths
	}
	if remaining == 0 || len(flexColumns) == 0 {
		return widths
	}

	type fractionalShare struct {
		column    int
		remainder int
	}
	shares := make([]fractionalShare, 0, len(flexColumns))
	distributed := 0
	for _, index := range flexColumns {
		weightedWidth := remaining * columns[index].Flex
		share := weightedWidth / flexWeight
		widths[index] += share
		distributed += share
		shares = append(shares, fractionalShare{column: index, remainder: weightedWidth % flexWeight})
	}
	sort.SliceStable(shares, func(i, j int) bool {
		return shares[i].remainder > shares[j].remainder
	})
	for _, share := range shares[:remaining-distributed] {
		widths[share.column]++
	}
	return widths
}

func shrinkColumnWidths(widths []int, columns []int, minimums []int, deficit int) int {
	for _, index := range columns {
		reduction := min(deficit*-1, widths[index]-minimums[index])
		if reduction > 0 {
			widths[index] -= reduction
			deficit += reduction
		}
		if deficit >= 0 {
			break
		}
	}
	return deficit
}

func widthsTotal(widths []int) int {
	total := 0
	for _, width := range widths {
		total += width
	}
	return total
}
