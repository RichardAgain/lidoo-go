package tui

import (
	"sort"
	"strings"

	bubblesTable "github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const dataTableCellPadding = 1

type dataTableColumn struct {
	Title    string
	Width    int // Fixed content width when Flex is zero; compressed only if necessary to fit.
	Flex     int // Relative share of unused width when positive.
	MinWidth int // Minimum content width for a flex column when space permits.
}

type dataTable struct {
	Columns     []dataTableColumn
	Rows        [][]string
	SelectedRow int
	Width       int
}

func (table dataTable) View() string {
	if len(table.Columns) == 0 {
		return ""
	}

	widths := responsiveColumnWidths(table.Columns, table.Width)
	columns := make([]bubblesTable.Column, len(table.Columns))
	for index, column := range table.Columns {
		columns[index] = bubblesTable.Column{Title: column.Title, Width: widths[index]}
	}
	rows := make([]bubblesTable.Row, len(table.Rows))
	for index, row := range table.Rows {
		rows[index] = bubblesTable.Row(row)
	}

	styles := bubblesTable.DefaultStyles()
	styles.Header = lipgloss.NewStyle().Bold(true).Padding(0, dataTableCellPadding)
	styles.Cell = lipgloss.NewStyle().Padding(0, dataTableCellPadding)
	styles.Selected = activeStyle
	component := bubblesTable.New(
		bubblesTable.WithStyles(styles),
		bubblesTable.WithColumns(columns),
		bubblesTable.WithRows(rows),
		bubblesTable.WithWidth(table.Width),
		bubblesTable.WithHeight(len(rows)+1),
	)
	component.SetCursor(table.SelectedRow)

	lines := strings.Split(component.View(), "\n")
	if len(lines) > 0 {
		lines = append(lines[:1], append([]string{mutedStyle.Render(strings.Repeat("─", max(0, table.Width)))}, lines[1:]...)...)
	}
	return fitDataTableWidth(strings.Join(lines, "\n"), table.Width)
}

// responsiveColumnWidths returns bubbles/table content widths. Its padding is
// added by the table styles, so subtract it before distributing the viewport.
func responsiveColumnWidths(columns []dataTableColumn, viewportWidth int) []int {
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

func fitDataTableWidth(view string, width int) string {
	if width <= 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	for index, line := range lines {
		line = ansi.Truncate(line, width, "")
		lines[index] = line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
	}
	return strings.Join(lines, "\n")
}
