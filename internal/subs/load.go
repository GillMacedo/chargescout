// Package subs finds subscriptions and other recurring charges in a bank statement.
package subs

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const maxFileBytes = 50_000_000

// Transaction is one statement row. Amount is in cents, and money out is negative.
type Transaction struct {
	Row         int
	Date        time.Time
	Amount      int64
	Description string
}

// Options controls how a statement is read.
type Options struct {
	Columns     map[string]string
	DateFormat  string
	PositiveOut bool
}

// Statement is a loaded file.
type Statement struct {
	Transactions []Transaction
	Skipped      []int
	Columns      map[string]string
	DateFormat   string
	Warnings     []string
}

var fieldNames = map[string][]string{
	"date":        {"date", "transaction date", "posted", "posted date", "posting date", "booking date", "value date", "data", "data lancamento", "fecha", "datum"},
	"amount":      {"amount", "transaction amount", "value", "valor", "importe", "betrag"},
	"credit":      {"credit", "credits", "paid in", "money in", "deposit", "deposits", "credito", "entrada"},
	"debit":       {"debit", "debits", "paid out", "money out", "withdrawal", "withdrawals", "debito", "saida"},
	"description": {"description", "details", "memo", "narrative", "payee", "merchant", "name", "transaction", "reference", "descricao", "historico", "concepto", "verwendungszweck"},
}

var accents = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "ç", "c", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ü", "u", "ä", "a", "ö", "o", "ñ", "n", "ß", "ss")

func normalHeader(cell string) string {
	cell = accents.Replace(strings.ToLower(strings.TrimSpace(cell)))
	return strings.Join(strings.Fields(strings.NewReplacer("_", " ", ".", " ", ":", " ").Replace(cell)), " ")
}

func detectColumns(header []string, overrides map[string]string) (map[string]int, error) {
	found := map[string]int{}
	for i, cell := range header {
		name := normalHeader(cell)
		for field, names := range fieldNames {
			for _, candidate := range names {
				if _, set := found[field]; !set && name == candidate {
					found[field] = i
				}
			}
		}
	}
	for field, name := range overrides {
		if _, known := fieldNames[field]; !known {
			return nil, fmt.Errorf("unknown field %q. Use date, amount, credit, debit or description", field)
		}
		index := -1
		for i, cell := range header {
			if normalHeader(cell) == normalHeader(name) {
				index = i
			}
		}
		if index < 0 {
			return nil, fmt.Errorf("no column named %q. Columns found: %s", name, strings.Join(header, ", "))
		}
		found[field] = index
	}
	return found, nil
}

func usable(found map[string]int) bool {
	_, date := found["date"]
	_, amount := found["amount"]
	_, credit := found["credit"]
	_, debit := found["debit"]
	_, description := found["description"]
	return date && description && (amount || credit || debit)
}

func decode(data []byte) string {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if utf8.Valid(data) {
		return string(data)
	}
	runes := make([]rune, len(data))
	for i, b := range data {
		runes[i] = rune(b)
	}
	return string(runes)
}

func detectDelimiter(text string) rune {
	lines := strings.Split(text, "\n")
	if len(lines) > 30 {
		lines = lines[:30]
	}
	best, bestCount := ',', -1
	for _, delimiter := range []rune{',', ';', '\t', '|'} {
		count := 0
		for _, line := range lines {
			count += strings.Count(line, string(delimiter))
		}
		if count > bestCount {
			best, bestCount = delimiter, count
		}
	}
	return best
}

var (
	amountNoise = regexp.MustCompile(`[A-Za-z$€£¥\s\x{00a0}']`)
	amountShape = regexp.MustCompile(`^[\d.,]*\d[\d.,]*$`)
	decimalPart = regexp.MustCompile(`[.,](\d{1,2})$`)
)

// ParseAmount returns cents. The last separator is the decimal mark only if one
// or two digits follow it, so both 1,234.56 and 1.234,56 work.
func ParseAmount(raw string) (int64, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, false
	}
	negative := false
	if strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") {
		negative, text = true, text[1:len(text)-1]
	}
	text = amountNoise.ReplaceAllString(text, "")
	if strings.HasPrefix(text, "-") || strings.HasPrefix(text, "−") {
		negative, text = !negative, strings.TrimLeft(text, "-−")
	} else if strings.HasSuffix(text, "-") {
		negative, text = !negative, strings.TrimRight(text, "-")
	}
	if !amountShape.MatchString(text) {
		return 0, false
	}
	fraction, whole := "", text
	if match := decimalPart.FindStringSubmatch(text); match != nil {
		fraction, whole = match[1], text[:len(text)-len(match[0])]
	}
	whole = strings.NewReplacer(".", "", ",", "").Replace(whole)
	if whole == "" {
		whole = "0"
	}
	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || units > 1<<52 {
		return 0, false
	}
	cents := units * 100
	if fraction != "" {
		value, _ := strconv.ParseInt((fraction + "0")[:2], 10, 64)
		cents += value
	}
	if negative {
		cents = -cents
	}
	return cents, true
}

var slashed = regexp.MustCompile(`^(\d{1,2})[/.\-](\d{1,2})[/.\-](\d{2}|\d{4})$`)
var isoDate = regexp.MustCompile(`^(\d{4})[/.\-](\d{1,2})[/.\-](\d{1,2})(?:[T ].*)?$`)

// DetectDateFormat reads the whole column. If every date could be day-first or
// month-first it returns "", because a guess would shift dates silently.
func DetectDateFormat(values []string) string {
	dayFirst, monthFirst, any := false, false, false
	for _, value := range values {
		match := slashed.FindStringSubmatch(strings.TrimSpace(value))
		if match == nil {
			continue
		}
		any = true
		first, _ := strconv.Atoi(match[1])
		second, _ := strconv.Atoi(match[2])
		dayFirst = dayFirst || first > 12
		monthFirst = monthFirst || second > 12
	}
	switch {
	case !any:
		return "ymd"
	case dayFirst == monthFirst:
		return ""
	case dayFirst:
		return "dmy"
	}
	return "mdy"
}

// ParseDate returns false for anything that isn't a real date.
func ParseDate(raw, format string) (time.Time, bool) {
	text := strings.TrimSpace(raw)
	var year, month, day int
	if match := isoDate.FindStringSubmatch(text); match != nil {
		year, _ = strconv.Atoi(match[1])
		month, _ = strconv.Atoi(match[2])
		day, _ = strconv.Atoi(match[3])
	} else if match := slashed.FindStringSubmatch(text); match != nil && (format == "dmy" || format == "mdy") {
		first, _ := strconv.Atoi(match[1])
		second, _ := strconv.Atoi(match[2])
		year, _ = strconv.Atoi(match[3])
		if year < 100 {
			year += 2000
		}
		day, month = first, second
		if format == "mdy" {
			day, month = second, first
		}
	} else {
		return time.Time{}, false
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return time.Time{}, false
	}
	return date, true
}

// Load reads a bank statement CSV.
func Load(path string, options Options) (*Statement, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("can't read %s: %w", path, errors.Unwrap(err))
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a folder. Pass a CSV file", path)
	}
	if info.Size() > maxFileBytes {
		return nil, fmt.Errorf("%s is larger than 50 MB", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("can't read %s: %w", path, err)
	}
	text := decode(data)
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = detectDelimiter(text)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("can't parse %s as CSV: %w", path, err)
	}

	headerIndex := -1
	var columns map[string]int
	for i := 0; i < len(rows) && i < 20; i++ {
		found, err := detectColumns(rows[i], options.Columns)
		if err != nil {
			if i == 0 && len(rows) > 0 && strings.Contains(err.Error(), "unknown field") {
				return nil, err
			}
			continue
		}
		if usable(found) {
			headerIndex, columns = i, found
			break
		}
	}
	if headerIndex < 0 {
		if len(options.Columns) > 0 && len(rows) > 0 {
			if _, err := detectColumns(rows[0], options.Columns); err != nil {
				return nil, err
			}
		}
		first := "(empty file)"
		if len(rows) > 0 {
			first = strings.Join(rows[0], ", ")
		}
		return nil, fmt.Errorf("can't find date, description and amount columns in %s. First row: %s. Name them with a column map", path, first)
	}

	cell := func(row []string, field string) string {
		if index, ok := columns[field]; ok && index < len(row) {
			return strings.TrimSpace(row[index])
		}
		return ""
	}
	body := rows[headerIndex+1:]
	var dates []string
	for _, row := range body {
		dates = append(dates, cell(row, "date"))
	}
	format := options.DateFormat
	if format == "" {
		format = DetectDateFormat(dates)
	}
	if format == "" {
		return nil, fmt.Errorf("the dates in %s could be day-first or month-first. Pass a date format of dmy or mdy", path)
	}

	statement := &Statement{DateFormat: format, Columns: map[string]string{}, Skipped: []int{}, Warnings: []string{}}
	for field, index := range columns {
		statement.Columns[field] = rows[headerIndex][index]
	}
	negatives := 0
	for offset, row := range body {
		number := headerIndex + offset + 2
		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue
		}
		date, ok := ParseDate(cell(row, "date"), format)
		if !ok {
			statement.Skipped = append(statement.Skipped, number)
			continue
		}
		amount, ok := ParseAmount(cell(row, "amount"))
		if !ok {
			credit, hasCredit := ParseAmount(cell(row, "credit"))
			debit, hasDebit := ParseAmount(cell(row, "debit"))
			if !hasCredit && !hasDebit {
				statement.Skipped = append(statement.Skipped, number)
				continue
			}
			if debit < 0 {
				debit = -debit
			}
			amount = credit - debit
		} else if options.PositiveOut {
			amount = -amount
		}
		if amount < 0 {
			negatives++
		}
		statement.Transactions = append(statement.Transactions, Transaction{Row: number, Date: date, Amount: amount, Description: cell(row, "description")})
	}
	if len(statement.Transactions) > 0 && negatives == 0 && !options.PositiveOut {
		statement.Warnings = append(statement.Warnings, "No row is money out. If this file shows charges as positive numbers, use the positive-out option.")
	}
	return statement, nil
}
