package reports

import "context"

type WorkbookRequest struct {
	FileName string  `json:"file_name"`
	Sheets   []Sheet `json:"sheets"`
}

type Sheet struct {
	Name      string   `json:"name"`
	HeaderRow int      `json:"header_row"`
	Columns   []Column `json:"columns"`
	Rows      []Row    `json:"rows"`
}

type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type Row struct {
	SourceRow        int               `json:"source_row"`
	Values           map[string]string `json:"values"`
	AutomationStatus string            `json:"-"`
}

type Generator interface {
	Generate(context.Context, *WorkbookRequest) ([]byte, error)
}
