package exporter

import (
	"encoding/csv"
	"os"
	"path/filepath"

	"sitesnap/internal/compare"
)

// ExportCSV exports a comparison report as multiple CSV files.
func ExportCSV(r *compare.Report, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	if err := writeAdded(filepath.Join(outDir, "added.csv"), r.Added); err != nil {
		return err
	}

	if err := writeRemoved(filepath.Join(outDir, "removed.csv"), r.Removed); err != nil {
		return err
	}

	if err := writeChanges(
		filepath.Join(outDir, "status_changes.csv"),
		r.StatusChanges,
	); err != nil {
		return err
	}

	if err := writeChanges(
		filepath.Join(outDir, "content_type_changes.csv"),
		r.ContentTypeChanges,
	); err != nil {
		return err
	}

	return nil
}

func writeAdded(path string, urls []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	w.Write([]string{"URL"})

	for _, u := range urls {
		w.Write([]string{u})
	}

	return w.Error()
}

func writeRemoved(path string, urls []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	w.Write([]string{"URL"})

	for _, u := range urls {
		w.Write([]string{u})
	}

	return w.Error()
}

func writeChanges(path string, changes []compare.Change) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	w.Write([]string{
		"URL",
		"Previous",
		"Current",
	})

	for _, c := range changes {
		w.Write([]string{
			c.URL,
			c.Previous,
			c.Current,
		})
	}

	return w.Error()
}
