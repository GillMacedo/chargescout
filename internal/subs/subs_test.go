package subs

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseAmount(t *testing.T) {
	cases := map[string]int64{"1,234.56": 123456, "1.234,56": 123456, "-15.99": -1599, "(50.00)": -5000, "50.00-": -5000, "$9.99": 999, "R$ 1.000,00": 100000, "12,5": 1250, "1,234": 123400, "0.10": 10}
	for raw, want := range cases {
		if got, ok := ParseAmount(raw); !ok || got != want {
			t.Errorf("%q: got %d", raw, got)
		}
	}
	for _, bad := range []string{"", "abc", "Closing"} {
		if _, ok := ParseAmount(bad); ok {
			t.Errorf("%q should fail", bad)
		}
	}
}

func TestDates(t *testing.T) {
	if DetectDateFormat([]string{"2026-03-02"}) != "ymd" || DetectDateFormat([]string{"25/02/2026"}) != "dmy" || DetectDateFormat([]string{"02/25/2026"}) != "mdy" {
		t.Error("detection failed")
	}
	if DetectDateFormat([]string{"03/02/2026", "04/02/2026"}) != "" || DetectDateFormat([]string{"25/02/2026", "02/25/2026"}) != "" {
		t.Error("ambiguous columns must not be guessed")
	}
	if date, ok := ParseDate("03/02/2026", "dmy"); !ok || date.Format("2006-01-02") != "2026-02-03" {
		t.Errorf("got %v", date)
	}
	if date, ok := ParseDate("03/02/26", "mdy"); !ok || date.Format("2006-01-02") != "2026-03-02" {
		t.Errorf("got %v", date)
	}
	for _, bad := range []string{"31/02/2026", "nope", ""} {
		if _, ok := ParseDate(bad, "dmy"); ok {
			t.Errorf("%q should fail", bad)
		}
	}
}

func TestMerchantName(t *testing.T) {
	cases := map[string]string{
		"SQ *SAMPLE STREAM 1004":                 "SAMPLE STREAM",
		"POS DEBIT EXAMPLE CLOUD STORAGE REF3":   "EXAMPLE CLOUD STORAGE",
		"PAYPAL *MOCKVIDEO 402-555-0100":         "MOCKVIDEO",
		"Fictional Gym 03/14":                    "FICTIONAL GYM",
		"VISA 1234 PURCHASE DEMO APP 2026-01-03": "DEMO APP",
		"www.sample-news.com":                    "SAMPLE NEWS",
		"12345":                                  "",
	}
	for raw, want := range cases {
		if got := MerchantName(raw); got != want {
			t.Errorf("%q: got %q, want %q", raw, got, want)
		}
	}
}

func date(text string) time.Time {
	parsed, err := time.Parse("2006-01-02", text)
	if err != nil {
		panic(err)
	}
	return parsed
}

func series(description string, cents int64, start string, stepDays, count int) []Transaction {
	var out []Transaction
	day := date(start)
	for i := 0; i < count; i++ {
		out = append(out, Transaction{Row: i + 2, Date: day, Amount: -cents, Description: description})
		day = day.AddDate(0, 0, stepDays)
	}
	return out
}

func monthly(description string, cents int64, count int) []Transaction {
	var out []Transaction
	for i := 0; i < count; i++ {
		out = append(out, Transaction{Row: i + 2, Date: date("2026-01-05").AddDate(0, i, 0), Amount: -cents, Description: description})
	}
	return out
}

func only(t *testing.T, r Result) Finding {
	t.Helper()
	if len(r.Findings) != 1 {
		t.Fatalf("want one finding, got %+v", r.Findings)
	}
	return r.Findings[0]
}

func TestCadences(t *testing.T) {
	cases := []struct {
		name string
		tx   []Transaction
	}{
		{"weekly", series("SAMPLE A", 500, "2026-01-01", 7, 6)},
		{"every 2 weeks", series("SAMPLE B", 500, "2026-01-01", 14, 6)},
		{"monthly", monthly("SAMPLE C", 500, 6)},
		{"quarterly", series("SAMPLE D", 500, "2025-01-01", 91, 5)},
		{"yearly", series("SAMPLE E", 500, "2024-01-01", 365, 3)},
	}
	for _, c := range cases {
		if got := only(t, Find(c.tx)); got.Cadence != c.name {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}

func TestConfidenceNeedsEnoughCharges(t *testing.T) {
	if got := only(t, Find(monthly("SAMPLE", 500, 4))); got.Confidence != Confirmed {
		t.Errorf("got %+v", got)
	}
	if got := only(t, Find(monthly("SAMPLE", 500, 3))); got.Confidence != Likely {
		t.Errorf("got %+v", got)
	}
	if r := Find(monthly("SAMPLE", 500, 2)); len(r.Findings) != 0 {
		t.Errorf("two monthly charges aren't enough: %+v", r.Findings)
	}
	if got := only(t, Find(series("SAMPLE", 500, "2025-01-01", 365, 2))); got.Cadence != "yearly" || got.Confidence != Likely {
		t.Errorf("got %+v", got)
	}
}

func TestIrregularSpendingIsNotASubscription(t *testing.T) {
	var shop []Transaction
	for i, gap := range []int{3, 40, 9, 22, 70, 5} {
		shop = append(shop, Transaction{Row: i, Date: date("2026-01-01").AddDate(0, 0, gap*(i+1)), Amount: -1000, Description: "DEMO SHOP"})
	}
	if r := Find(shop); len(r.Findings) != 0 {
		t.Errorf("got %+v", r.Findings)
	}
	varying := []Transaction{}
	for i, cents := range []int64{1000, 5000, 200, 9000, 40} {
		varying = append(varying, Transaction{Row: i, Date: date("2026-01-05").AddDate(0, i, 0), Amount: -cents, Description: "DEMO BAKERY"})
	}
	if r := Find(varying); len(r.Findings) != 0 {
		t.Errorf("wildly varying amounts: %+v", r.Findings)
	}
}

func TestMoneyInIsIgnored(t *testing.T) {
	tx := monthly("CUSTOMER PAYMENT", 500, 6)
	for i := range tx {
		tx[i].Amount = -tx[i].Amount
	}
	if r := Find(tx); len(r.Findings) != 0 {
		t.Errorf("got %+v", r.Findings)
	}
}

func TestPriceIncreaseIsReported(t *testing.T) {
	tx := append(monthly("SAMPLE STREAM", 1599, 3), Transaction{Row: 9, Date: date("2026-04-05"), Amount: -1899, Description: "SAMPLE STREAM"})
	got := only(t, Find(tx))
	if len(got.Alerts) != 1 || !strings.Contains(got.Alerts[0], "from 15.99 to 18.99 on 2026-04-05") {
		t.Errorf("got %+v", got.Alerts)
	}
	if got.LastAmount != "18.99" || got.YearlyCost != "227.88" {
		t.Errorf("got %+v", got)
	}
}

func TestDriftingAmountsDontChainTogether(t *testing.T) {
	var tx []Transaction
	for i := 0; i < 9; i++ {
		tx = append(tx, Transaction{Row: i, Date: date("2026-01-05").AddDate(0, i, 0), Amount: -int64(1250 + i*300), Description: "DEMO SHOP"})
	}
	for _, group := range clusters(tx) {
		if first, last := abs(group[0].Amount), abs(group[len(group)-1].Amount); float64(last) > float64(first)*1.25 {
			t.Errorf("group spans %d to %d", first, last)
		}
	}
}

func TestVariableBillsAreLikelyWithoutPriceAlerts(t *testing.T) {
	var tx []Transaction
	for i, cents := range []int64{8000, 8400, 7900, 8700, 8100, 8600} {
		tx = append(tx, Transaction{Row: i, Date: date("2026-01-05").AddDate(0, i, 0), Amount: -cents, Description: "SAMPLE POWER"})
	}
	got := only(t, Find(tx))
	if got.Confidence != Likely || len(got.Alerts) != 1 || !strings.Contains(got.Alerts[0], "usage-based") {
		t.Errorf("got %+v", got)
	}
}

func TestDoubleChargeIsReported(t *testing.T) {
	tx := append(monthly("EXAMPLE CLOUD", 999, 5), Transaction{Row: 9, Date: date("2026-03-06"), Amount: -999, Description: "EXAMPLE CLOUD"})
	got := only(t, Find(tx))
	if got.Cadence != "monthly" || len(got.Alerts) != 1 || !strings.Contains(got.Alerts[0], "double charge of 9.99 on 2026-03-06") {
		t.Errorf("got %+v", got)
	}
}

func TestTwoPlansFromOneMerchantStaySeparate(t *testing.T) {
	r := Find(append(monthly("SAMPLE APPS", 199, 5), monthly("SAMPLE APPS", 2999, 5)...))
	if len(r.Findings) != 2 || r.Findings[0].LastAmount != "29.99" || r.Findings[1].LastAmount != "1.99" {
		t.Errorf("got %+v", r.Findings)
	}
}

func TestStoppedChargesLeaveTheTotal(t *testing.T) {
	tx := append(monthly("FICTIONAL GYM", 4500, 4), monthly("SAMPLE STREAM", 1000, 12)...)
	r := Find(tx)
	if len(r.Findings) != 2 || r.ActiveCount != 1 || r.ActiveYearly != "120.00" {
		t.Fatalf("got %+v", r)
	}
	if r.Findings[1].Status != "may have stopped" || r.Findings[1].NextDate != "" {
		t.Errorf("got %+v", r.Findings[1])
	}
	if r.Findings[0].NextDate == "" {
		t.Errorf("got %+v", r.Findings[0])
	}
}

func TestInvariantsOnRandomData(t *testing.T) {
	random := rand.New(rand.NewSource(7))
	names := []string{"SAMPLE A", "SAMPLE B", "DEMO SHOP", "MOCK", "12345", "POS DEBIT"}
	for round := 0; round < 400; round++ {
		var tx []Transaction
		for i := 0; i < random.Intn(40); i++ {
			tx = append(tx, Transaction{Row: i, Date: date("2025-01-01").AddDate(0, 0, random.Intn(600)), Amount: int64(random.Intn(4000) - 3000), Description: names[random.Intn(len(names))]})
		}
		r := Find(tx)
		for _, finding := range r.Findings {
			if finding.Merchant == "" || finding.Charges < 2 {
				t.Fatalf("bad finding %+v", finding)
			}
			if finding.Status == "active" && finding.NextDate == "" {
				t.Fatalf("active without next date %+v", finding)
			}
		}
		if fmt.Sprint(r) != fmt.Sprint(Find(tx)) {
			t.Fatal("not deterministic")
		}
	}
	if r := Find(nil); len(r.Findings) != 0 || r.ActiveYearly != "0.00" {
		t.Errorf("empty: %+v", r)
	}
}

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file.csv")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFormats(t *testing.T) {
	statement, err := Load(write(t, "Statement,,,\nDate,Details,Credit,Debit\n2026-03-02,IN,\"1,200.00\",\n2026-03-03,OUT,,50.00\nClosing,,,\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(statement.Transactions) != 2 || statement.Transactions[0].Amount != 120000 || statement.Transactions[1].Amount != -5000 || len(statement.Skipped) != 1 {
		t.Errorf("got %+v", statement)
	}
	statement, err = Load(write(t, "Data;Hist\xf3rico;Valor\n25/02/2026;PIX;-1.234,56\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if statement.Transactions[0].Amount != -123456 || statement.DateFormat != "dmy" || statement.Columns["description"] != "Histórico" {
		t.Errorf("got %+v", statement)
	}
}

func TestPositiveOut(t *testing.T) {
	path := write(t, "Date,Description,Amount\n2026-03-02,SAMPLE,15.00\n")
	statement, err := Load(path, Options{})
	if err != nil || len(statement.Warnings) != 1 || !strings.Contains(statement.Warnings[0], "positive-out") {
		t.Errorf("%v %+v", err, statement)
	}
	statement, err = Load(path, Options{PositiveOut: true})
	if err != nil || statement.Transactions[0].Amount != -1500 || len(statement.Warnings) != 0 {
		t.Errorf("%v %+v", err, statement)
	}
}

func TestLoadErrors(t *testing.T) {
	ambiguous := write(t, "Date,Description,Amount\n03/02/2026,A,-1\n04/02/2026,B,-2\n")
	if _, err := Load(ambiguous, Options{}); err == nil || !strings.Contains(err.Error(), "dmy or mdy") {
		t.Errorf("got %v", err)
	}
	if statement, err := Load(ambiguous, Options{DateFormat: "dmy"}); err != nil || statement.Transactions[0].Date.Format("2006-01-02") != "2026-02-03" {
		t.Errorf("%v %+v", err, statement)
	}
	unknown := write(t, "Foo,Bar\n1,2\n")
	if _, err := Load(unknown, Options{}); err == nil || !strings.Contains(err.Error(), "First row: Foo, Bar") {
		t.Errorf("got %v", err)
	}
	if _, err := Load(unknown, Options{Columns: map[string]string{"date": "Nope"}}); err == nil || !strings.Contains(err.Error(), "Columns found: Foo, Bar") {
		t.Errorf("got %v", err)
	}
	if _, err := Load("/no/such.csv", Options{}); err == nil {
		t.Error("missing file should fail")
	}
	if _, err := Load(t.TempDir(), Options{}); err == nil {
		t.Error("folder should fail")
	}
	custom := write(t, "When,Sum,What\n2026-03-02,-5,X\n")
	if statement, err := Load(custom, Options{Columns: map[string]string{"date": "When", "amount": "Sum", "description": "What"}}); err != nil || len(statement.Transactions) != 1 {
		t.Errorf("%v %+v", err, statement)
	}
}
