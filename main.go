package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"github.com/jung-kurt/gofpdf"
	"github.com/ledongthuc/pdf"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed template/index.html
var content embed.FS

var pdfContent strings.Builder
var tpl = template.Must(template.ParseFiles("template/index.html"))

func init() {
	data, err := content.ReadFile("template/index.html")
	if err != nil {
		log.Fatalf("Failed to read embedded page.html: %v", err)
	}
	tpl = template.Must(template.New("page.html").Parse(string(data)))
}

func toUTF8(text string) (string, error) {
	if utf8.ValidString(text) {
		return text, nil
	}

	reader := transform.NewReader(strings.NewReader(text), charmap.ISO8859_1.NewDecoder())
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(reader)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func normalizeSpaces(text string) string {
	var result strings.Builder
	for _, r := range text {
		if unicode.IsSpace(r) {
			result.WriteRune(' ')
		} else {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

func readPdfContent(reader *pdf.Reader) {
	totalPages := reader.NumPage()

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := reader.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			fmt.Printf("Error on page num %d, %v\n", pageNum, err)
			continue
		}

		utf8Text, err := toUTF8(text)
		if err != nil {
			log.Printf("Error converting to UTF-8 on page %d: %v\n", pageNum, err)
			continue
		}

		normalizedText := normalizeSpaces(utf8Text)
		pdfFormatedContent := fmt.Sprintf("Page -> %d \n%s\n", pageNum, normalizedText)
		pdfContent.WriteString(pdfFormatedContent)

	}
}

func writeToPdf(content strings.Builder) *gofpdf.Fpdf {
	fpdf := gofpdf.New("P", "mm", "A4", "")
	fpdf.AddPage()

	fpdf.SetMargins(20, 20, 20)

	fpdf.AddUTF8Font("georgia", "", "./georgia.ttf")
	fpdf.AddUTF8Font("georgiab", "B", "./georgiab.ttf")

	fpdf.SetFont("georgia", "", 12)

	text := content.String()
	paragraphs := strings.Split(text, "\n\n")
	for _, paragraph := range paragraphs {
		lines := strings.Split(strings.TrimSpace(paragraph), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}

			words := strings.Fields(line)
			for i, word := range words {
				if len(word) == 0 {
					continue
				}

				firstRune, size := utf8.DecodeRuneInString(word)
				if firstRune == utf8.RuneError {
					log.Printf("Invalid UTF-8 rune in word: %s\n", word)
					continue
				}
				firstLetter := string(firstRune)
				restOfWord := word[size:]

				fpdf.SetFont("georgiab", "B", 12)
				fpdf.Write(5, firstLetter)

				fpdf.SetFont("georgia", "", 12)
				fpdf.Write(5, restOfWord)

				if i < len(words)-1 {
					fpdf.Write(5, " ")
				}
			}
			fpdf.Ln(5)
		}
		fpdf.Ln(10)
	}

	fpdf.SetHeaderFunc(func() {
		fpdf.SetFont("georgia", "", 12)
	})

	return fpdf
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		tpl.Execute(w, nil)
		return
	}

	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("pdfFile")
	if err != nil {
		http.Error(w, "Unable to get file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Unable to read file", http.StatusInternalServerError)
		return
	}

	tmpFile, err := os.CreateTemp("", "upload_*.pdf")
	if err != nil {
		http.Error(w, "Unable to create temp file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, err = tmpFile.Write(fileBytes)
	if err != nil {
		http.Error(w, "Unable to write temp file", http.StatusInternalServerError)
		return
	}

	f, rr, err := pdf.Open(tmpFile.Name())
	if err != nil {
		http.Error(w, "Unable to open PDF", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	pdfContent.Reset()
	readPdfContent(rr)

	fpdf := writeToPdf(pdfContent)

	var buf bytes.Buffer
	err = fpdf.Output(&buf)
	if err != nil {
		http.Error(w, "Unable to generate PDF", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=\"book.pdf\"")
	w.Header().Set("Content-Length", fmt.Sprint(buf.Len()))

	w.Write(buf.Bytes())
}

func main() {
	http.HandleFunc("/", uploadHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Server listening on http://localhost:" + port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("Error starting server:", err)
	}
}
