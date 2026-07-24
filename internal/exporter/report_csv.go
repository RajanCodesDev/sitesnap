package exporter

import (
	"encoding/csv"
	"os"

	"github.com/RajanCodesDev/sitesnap/internal/compare"
)

// ExportReportCSV exports the deployment comparison as a single CSV file.
func ExportReportCSV(r *compare.Report, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{
		"Type",
		"URL",
		"Previous",
		"Current",
	}); err != nil {
		return err
	}

	for _, u := range r.Added {
		if err := w.Write([]string{
			"Added",
			u,
			"",
			"",
		}); err != nil {
			return err
		}
	}

	for _, u := range r.Removed {
		if err := w.Write([]string{
			"Removed",
			u,
			"",
			"",
		}); err != nil {
			return err
		}
	}

	for _, c := range r.StatusChanges {
		if err := w.Write([]string{
			"Status Change",
			c.URL,
			c.Previous,
			c.Current,
		}); err != nil {
			return err
		}
	}

	for _, c := range r.ContentTypeChanges {
		if err := w.Write([]string{
			"Content-Type Change",
			c.URL,
			c.Previous,
			c.Current,
		}); err != nil {
			return err
		}
	}

	return w.Error()
}
