package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"elcome-invoice-parser/pkg/elcomeinvoice"
)

func main() {
	var pdfPath string
	flag.StringVar(&pdfPath, "pdf", "", "Path to Elcome invoice PDF")
	flag.Parse()
	if pdfPath == "" {
		if flag.NArg() > 0 {
			pdfPath = flag.Arg(0)
		}
	}
	if pdfPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: elcome-invoice-parser --pdf <file.pdf>\n   or: elcome-invoice-parser <file.pdf>")
		os.Exit(2)
	}

	inv, err := elcomeinvoice.ParsePDF(pdfPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(inv); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
