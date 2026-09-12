package report

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/jung-kurt/gofpdf"
)

type RepoSummary struct {
	RepoSlug string
	Summary  string
}

// sanitize removes characters the built-in PDF fonts can't render
// (emoji, smart quotes, dashes, etc.) and normalizes markdown to ASCII.
func sanitize(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch r {
		case '\u2018', '\u2019', '\u201C', '\u201D', '\u201A', '\u201B':
			b.WriteRune('\'')
		case '\u2013', '\u2014', '\u2212':
			b.WriteRune('-')
		case '\u2022', '\u25E6', '\u2043':
			b.WriteRune('*')
		case '\u00A0':
			b.WriteRune(' ')
		default:
			if r < 128 {
				b.WriteRune(r)
			} else if unicode.IsSpace(r) {
				b.WriteRune(' ')
			}
		}
	}
	return b.String()
}

// cleanInlineMarkdown strips markdown emphasis markers and code ticks.
func cleanInlineMarkdown(text string) string {
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "`", "")
	return text
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

		lines := strings.Split(s.Summary, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			switch {
			case strings.HasPrefix(trimmed, "##### "):
				pdf.SetFont("Helvetica", "B", 11)
				text := cleanInlineMarkdown(strings.TrimPrefix(trimmed, "##### "))
				pdf.MultiCell(0, 6, sanitize(text), "", "L", false)
				pdf.Ln(1)
			case strings.HasPrefix(trimmed, "### "):
				pdf.SetFont("Helvetica", "B", 13)
				text := cleanInlineMarkdown(strings.TrimPrefix(trimmed, "### "))
				pdf.MultiCell(0, 7, sanitize(text), "", "L", false)
				pdf.Ln(1)
			case strings.HasPrefix(trimmed, "#### "):
				pdf.SetFont("Helvetica", "B", 12)
				text := cleanInlineMarkdown(strings.TrimPrefix(trimmed, "#### "))
				pdf.MultiCell(0, 6, sanitize(text), "", "L", false)
				pdf.Ln(1)
			case strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "- "):
				pdf.SetFont("Helvetica", "", 11)
				rawContent := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "*"), "-"))
				text := cleanInlineMarkdown(rawContent)
				pdf.MultiCell(0, 6, "   -  "+sanitize(text), "", "L", false)
			default:
				pdf.SetFont("Helvetica", "", 11)
				text := cleanInlineMarkdown(trimmed)
				pdf.MultiCell(0, 6, sanitize(text), "", "L", false)
			}
		}
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