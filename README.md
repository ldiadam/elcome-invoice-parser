# elcome-invoice-parser

Parse Elcome PDF invoices using Poppler `pdftotext` + regex.

## Requirements

- `pdftotext` (Poppler). On Ubuntu:
  - `sudo apt-get install -y poppler-utils`

## Usage

```bash
go run ./cmd/elcome-invoice-parser --pdf /path/to/invoice.pdf
```

Outputs JSON to stdout.
