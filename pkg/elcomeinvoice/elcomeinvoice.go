package elcomeinvoice

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type Invoice struct {
	InvoiceNumber string `json:"invoice_number"`
	InvoiceDate   string `json:"invoice_date"`
	DueDate       string `json:"due_date"`
	PaymentTerms  string `json:"payment_terms"`

	Currency      string  `json:"currency"`
	InvoiceAmount float64 `json:"invoice_amount"`
	AmountDue     float64 `json:"amount_due"`

	CustomerID      string `json:"customer_id"`
	SubscriptionID  string `json:"subscription_id"`
	BillingPeriod   string `json:"billing_period"`
	VesselOrVehicle string `json:"vessel_or_vehicle"`
	SerialNumber    string `json:"serial_number"`

	Supplier Supplier   `json:"supplier"`
	BilledTo BilledTo   `json:"billed_to"`
	Items    []LineItem `json:"items,omitempty"`

	RawHints map[string]string `json:"raw_hints,omitempty"`
}

type Supplier struct {
	Name   string `json:"name"`
	VAT    string `json:"vat_eori"`
	Street string `json:"address,omitempty"`
}

type BilledTo struct {
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`
	Address string `json:"address,omitempty"`
}

type LineItem struct {
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Discount    float64 `json:"discount"`
	Amount      float64 `json:"amount"`
}

// Runner allows swapping out the PDF-to-text extraction method.
// Default runner uses Poppler's pdftotext.
type Runner interface {
	PDFToText(pdfPath string) (string, error)
}

type PdftotextRunner struct{}

func (r PdftotextRunner) PDFToText(pdfPath string) (string, error) {
	cmd := exec.Command("pdftotext", pdfPath, "-")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}

func ParsePDF(pdfPath string) (*Invoice, error) {
	return ParsePDFWithRunner(pdfPath, PdftotextRunner{})
}

func ParsePDFWithRunner(pdfPath string, runner Runner) (*Invoice, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		if _, ok := runner.(PdftotextRunner); ok {
			return nil, fmt.Errorf("pdftotext not found; install poppler-utils")
		}
	}
	text, err := runner.PDFToText(pdfPath)
	if err != nil {
		return nil, err
	}
	return ParseText(text)
}

func ParseText(text string) (*Invoice, error) {
	return parseInvoice(text)
}

func findFirst(re *regexp.Regexp, text string) string {
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func mustFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	return strconv.ParseFloat(s, 64)
}

func parseMoney(s string) (float64, string) {
	s = strings.TrimSpace(s)
	// examples: $225.00 (USD), -$25.00, $250.00
	re := regexp.MustCompile(`([+-]?)\$([0-9]+(?:\.[0-9]{2})?)`)
	m := re.FindStringSubmatch(s)
	if len(m) < 3 {
		return 0, ""
	}
	sign := m[1]
	amt := m[2]
	v, err := mustFloat(amt)
	if err != nil {
		return 0, ""
	}
	if sign == "-" {
		v = -v
	}
	cur := ""
	m2 := regexp.MustCompile(`\((USD|EUR|SGD|IDR)\)`).FindStringSubmatch(s)
	if len(m2) >= 2 {
		cur = m2[1]
	}
	return v, cur
}

func parseInvoice(text string) (*Invoice, error) {
	all := text
	lines := strings.Split(all, "\n")

	inv := &Invoice{RawHints: map[string]string{}}

	inv.InvoiceNumber = findFirst(regexp.MustCompile(`(?m)^Invoice #\s+([A-Z0-9-]+)\s*$`), all)
	inv.InvoiceDate = findFirst(regexp.MustCompile(`(?m)^Invoice Date\s+([A-Za-z]{3} [0-9]{2}, [0-9]{4})\s*$`), all)
	inv.DueDate = findFirst(regexp.MustCompile(`(?m)^Due Date\s+([A-Za-z]{3} [0-9]{2}, [0-9]{4})\s*$`), all)
	inv.PaymentTerms = findFirst(regexp.MustCompile(`(?m)^Payment Terms\s+(.+)\s*$`), all)
	inv.CustomerID = findFirst(regexp.MustCompile(`(?m)^Customer ID\s+([0-9]+)\s*$`), all)

	// Invoice amount line contains currency.
	amtLine := findFirst(regexp.MustCompile(`(?m)^Invoice Amount\s+(.+)\s*$`), all)
	if amtLine != "" {
		v, cur := parseMoney(amtLine)
		if cur != "" {
			inv.Currency = cur
		}
		inv.InvoiceAmount = v
	}

	dueLine := findFirst(regexp.MustCompile(`(?m)^Amount Due \(USD\)\s+\$([0-9]+\.[0-9]{2})\s*$`), all)
	if dueLine != "" {
		if v, err := mustFloat(dueLine); err == nil {
			inv.AmountDue = v
		}
		if inv.Currency == "" {
			inv.Currency = "USD"
		}
	}

	// Supplier
	inv.Supplier.Name = findFirst(regexp.MustCompile(`(?m)^Elcome Europe\s+S\.L\.$`), all)
	if inv.Supplier.Name == "" {
		inv.Supplier.Name = findFirst(regexp.MustCompile(`(?m)^([A-Za-z].*\bS\.L\.)\s*$`), all)
	}
	inv.Supplier.VAT = findFirst(regexp.MustCompile(`(?m)^VAT/EORI:\s*(\S+)\s*$`), all)
	inv.Supplier.Street = strings.TrimSpace(strings.Join(findSupplierAddress(lines), "\n"))

	// Billed to block
	inv.BilledTo = parseBilledTo(lines)

	inv.SubscriptionID = findFirst(regexp.MustCompile(`(?m)^ID\s+([A-Za-z0-9]+)\s*$`), all)
	inv.BillingPeriod = findFirst(regexp.MustCompile(`(?m)^Billing Period\s+(.+)\s*$`), all)
	inv.VesselOrVehicle = findFirst(regexp.MustCompile(`(?m)^Vessel / Vehicle\s+(.+)\s*$`), all)
	inv.SerialNumber = findFirst(regexp.MustCompile(`(?m)^Serial Number\s+(.+)\s*$`), all)

	inv.Items = parseLineItems(lines)

	if inv.Currency == "" {
		inv.Currency = "USD" // per sample
	}

	if inv.InvoiceNumber == "" {
		return nil, fmt.Errorf("could not parse invoice number")
	}
	return inv, nil
}

func findSupplierAddress(lines []string) []string {
	// Immediately after the supplier name line and before VAT/EORI.
	out := []string{}
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(l), "Elcome Europe") {
			start = i + 1
			continue
		}
		if start >= 0 && i >= start {
			if strings.HasPrefix(strings.TrimSpace(l), "VAT/EORI") {
				break
			}
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

func parseBilledTo(lines []string) BilledTo {
	// Look for the BILLED TO section.
	var b BilledTo
	idx := -1
	for i, l := range lines {
		if strings.TrimSpace(strings.ToUpper(l)) == "BILLED TO" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return b
	}

	// The first meaningful non-empty non-header line is the company name.
	i := idx + 1
	for i < len(lines) {
		l := strings.TrimSpace(lines[i])
		if l == "" {
			i++
			continue
		}
		up := strings.ToUpper(l)
		// Some PDFs place a "SUBSCRIPTION" header between BILLED TO and the actual name.
		if up == "SUBSCRIPTION" || up == "POSTED" {
			i++
			continue
		}
		b.Name = l
		i++
		break
	}

	// Collect address/email/phone until we hit subscription details or the line-item section.
	addrLines := []string{}
	for i < len(lines) {
		l := strings.TrimSpace(lines[i])
		if l == "" {
			i++
			continue
		}
		up := strings.ToUpper(l)
		if up == "ID" || strings.HasPrefix(up, "ID ") || strings.HasPrefix(up, "BILLING PERIOD") || strings.HasPrefix(up, "VESSEL / VEHICLE") || strings.HasPrefix(up, "SERIAL NUMBER") || up == "DESCRIPTION" {
			break
		}
		if strings.Contains(l, "@") { // email
			b.Email = l
			i++
			continue
		}
		if strings.HasPrefix(l, "+") {
			b.Phone = l
			i++
			continue
		}
		addrLines = append(addrLines, l)
		i++
	}
	b.Address = strings.Join(addrLines, ", ")
	return b
}

func parseLineItems(lines []string) []LineItem {
	// Find header "DESCRIPTION" then read next description and the money lines.
	idx := -1
	for i, l := range lines {
		if strings.TrimSpace(strings.ToUpper(l)) == "DESCRIPTION" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}

	// Scan forward until we hit "Total".
	items := []LineItem{}
	i := idx + 1
	for i < len(lines) {
		l := strings.TrimSpace(lines[i])
		if strings.EqualFold(l, "Total") {
			break
		}
		if l == "" || l == "PRICE" || l == "DISCOUNT" || strings.HasPrefix(l, "AMOUNT") {
			i++
			continue
		}

		// first non-empty line is description
		desc := l
		// look ahead for a line containing two money values: price and discount
		price := 0.0
		disc := 0.0
		amount := 0.0
		j := i + 1
		for j < len(lines) {
			ll := strings.TrimSpace(lines[j])
			if strings.EqualFold(ll, "Total") {
				break
			}
			if ll == "" {
				j++
				continue
			}
			// match "$250.00 -$25.00"
			reTwo := regexp.MustCompile(`\$[0-9]+\.[0-9]{2}\s+-\$[0-9]+\.[0-9]{2}`)
			if reTwo.MatchString(ll) {
				parts := strings.Fields(ll)
				if len(parts) >= 2 {
					p, _ := parseMoney(parts[0])
					d, _ := parseMoney(parts[1])
					price = p
					disc = d
				}
				j++
				continue
			}
			// amount line "$225.00"
			if strings.HasPrefix(ll, "$") && !strings.Contains(ll, "(") {
				if v, _ := parseMoney(ll); v != 0 {
					amount = v
				}
				// we've got enough
				j++
				break
			}
			j++
		}

		if desc != "" && (price != 0 || amount != 0 || disc != 0) {
			items = append(items, LineItem{Description: desc, Price: price, Discount: disc, Amount: amount})
		}
		i = j
	}
	return items
}
