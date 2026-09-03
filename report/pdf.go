package report

import (
	"fmt"
	"os"
	"time"

	"github.com/jung-kurt/gofpdf"
)

type RepoSummary struct {
	RepoSlug string
	Summary  string
}

func Generate(summaries []RepoSummary, startDate, endDate time.Time, outputPath string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 24)
	pdf.CellFormat(0, 15, "Weekly Tech Team Report", "", 1, "C", false, 0, "")
	pdf.Ln(5)

	pdf.SetFont("Helvetica", "", 12)
	dateRange := fmt.Sprintf("%s to %s",
		startDate.Format("January 2, 2006"),
		endDate.Format("January 2, 2006"))
	pdf.CellFormat(0, 10, dateRange, "", 1, "C", false, 0, "")
	pdf.Ln(10)

	for _, s := range summaries {
		pdf.SetFont("Helvetica", "B", 16)
		pdf.SetFillColor(240, 240, 240)
		pdf.CellFormat(0, 10, "  "+s.RepoSlug, "", 1, "L", true, 0, "")
		pdf.Ln(3)

		pdf.SetFont("Helvetica", "", 11)
		pdf.MultiCell(0, 6, s.Summary, "", "L", false)
		pdf.Ln(8)
	}

	pdf.SetFont("Helvetica", "I", 9)
	pdf.SetTextColor(128, 128, 128)
	pdf.CellFormat(0, 10,
		fmt.Sprintf("Generated on %s", time.Now().Format("January 2, 2006 at 3:04 PM")),
		"", 1, "R", false, 0, "")

	return pdf.OutputFileAndClose(outputPath)
}

func EnsureOutputDir(dir string) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	return nil
}
