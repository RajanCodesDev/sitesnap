package exporter

import (
	"encoding/csv"
	"fmt"
	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
	"os"
	"strconv"
)

func ExportSnapshotCSV(s *snapshot.Snapshot, path string) error {
	fmt.Printf("Exporting %d pages to %s\n", len(s.Pages), path)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{
		"URL",
		"Status Code",
		"Content Type",
	}); err != nil {
		return err
	}

	for _, p := range s.Pages {
		if err := w.Write([]string{
			p.URL,
			statusText(p.StatusCode),
			p.ContentType,
		}); err != nil {
			return err
		}
	}

	return w.Error()
}

func statusText(code int) string {
	if code == 0 {
		return "unreachable"
	}
	return strconv.Itoa(code)
}
