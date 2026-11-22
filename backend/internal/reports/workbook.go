package reports

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"
)

func writeWorkbook(request *WorkbookRequest) ([]byte, error) {
	workbook := excelize.NewFile()
	defer workbook.Close()

	usedNames := make(map[string]bool, len(request.Sheets))
	for sheetIndex := range request.Sheets {
		sheet := &request.Sheets[sheetIndex]
		sheetName := uniqueSheetName(sheet.Name, usedNames)
		if sheetIndex == 0 {
			if err := workbook.SetSheetName("Sheet1", sheetName); err != nil {
				return nil, fmt.Errorf("name report sheet: %w", err)
			}
		} else if _, err := workbook.NewSheet(sheetName); err != nil {
			return nil, fmt.Errorf("create report sheet: %w", err)
		}

		statusColumn := len(sheet.Columns) + 1
		for columnIndex, column := range sheet.Columns {
			if column.Key == "automation_status" {
				statusColumn = columnIndex + 1
				break
			}
		}

		for columnIndex, column := range sheet.Columns {
			label := column.Label
			if column.Key == "automation_status" {
				label = "automation_status"
			}
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, sheet.HeaderRow)
			if err != nil {
				return nil, fmt.Errorf("locate report header cell: %w", err)
			}
			if err := workbook.SetCellValue(sheetName, cell, label); err != nil {
				return nil, fmt.Errorf("write report header: %w", err)
			}
		}
		if statusColumn > len(sheet.Columns) {
			cell, err := excelize.CoordinatesToCellName(statusColumn, sheet.HeaderRow)
			if err != nil {
				return nil, fmt.Errorf("locate automation status header: %w", err)
			}
			if err := workbook.SetCellValue(sheetName, cell, "automation_status"); err != nil {
				return nil, fmt.Errorf("write automation status header: %w", err)
			}
		}

		for _, row := range sheet.Rows {
			for columnIndex, column := range sheet.Columns {
				value := row.Values[column.Key]
				if column.Key == "automation_status" {
					value = row.AutomationStatus
				}
				cell, err := excelize.CoordinatesToCellName(columnIndex+1, row.SourceRow)
				if err != nil {
					return nil, fmt.Errorf("locate report data cell: %w", err)
				}
				if err := workbook.SetCellValue(sheetName, cell, value); err != nil {
					return nil, fmt.Errorf("write report data: %w", err)
				}
			}
			if statusColumn > len(sheet.Columns) {
				cell, err := excelize.CoordinatesToCellName(statusColumn, row.SourceRow)
				if err != nil {
					return nil, fmt.Errorf("locate automation status cell: %w", err)
				}
				if err := workbook.SetCellValue(sheetName, cell, row.AutomationStatus); err != nil {
					return nil, fmt.Errorf("write automation status: %w", err)
				}
			}
		}
	}

	buffer, err := workbook.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("encode report workbook: %w", err)
	}
	return buffer.Bytes(), nil
}

func uniqueSheetName(input string, used map[string]bool) string {
	var builder strings.Builder
	for _, character := range strings.TrimSpace(input) {
		switch character {
		case '[', ']', ':', '*', '?', '/', '\\':
			builder.WriteByte('_')
		default:
			if unicode.IsControl(character) {
				builder.WriteByte('_')
			} else {
				builder.WriteRune(character)
			}
		}
	}
	base := strings.Trim(builder.String(), "'")
	if base == "" {
		base = "Sheet"
	}
	baseRunes := []rune(base)
	if len(baseRunes) > 31 {
		base = string(baseRunes[:31])
	}
	name := base
	for suffix := 2; used[strings.ToLower(name)]; suffix++ {
		suffixText := fmt.Sprintf(" (%d)", suffix)
		prefixLength := 31 - len([]rune(suffixText))
		baseRunes := []rune(base)
		if len(baseRunes) > prefixLength {
			name = string(baseRunes[:prefixLength]) + suffixText
		} else {
			name = base + suffixText
		}
	}
	used[strings.ToLower(name)] = true
	return name
}
