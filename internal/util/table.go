package util

import (
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
)

type TableWriter interface {
	AppendHeader(row table.Row, configs ...table.RowConfig)
	AppendRow(row table.Row, configs ...table.RowConfig)
	Render() string
}

func GetTableWriter() table.Writer {
	t := table.NewWriter()
	t.SetStyle(table.StyleDefault)
	t.SetOutputMirror(os.Stdout)
	return t
}
