package subs

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Confidence says how sure a finding is.
const (
	Confirmed = "recurring"
	Likely    = "likely"
)

const (
	minLikelyCharges    = 3
	minConfirmedCharges = 4
	sameAmountTolerance = 0.25
	onTimeShare         = 0.75
	duplicateWithinDays = 3
)

type cadence struct {
	name      string
	days      float64
	tolerance float64
	perYear   float64
}

var cadences = []cadence{
	{"weekly", 7, 2, 52},
	{"every 2 weeks", 14, 3, 26},
	{"monthly", 30.44, 5, 12},
	{"quarterly", 91.3, 12, 4},
	{"yearly", 365.25, 20, 1},
}

// Finding is one recurring charge.
type Finding struct {
	Merchant    string   `json:"merchant"`
	Cadence     string   `json:"cadence"`
	Confidence  string   `json:"confidence"`
	Status      string   `json:"status"`
	Charges     int      `json:"charges"`
	LastAmount  string   `json:"last_amount"`
	LastDate    string   `json:"last_date"`
	FirstDate   string   `json:"first_date"`
	NextDate    string   `json:"next_expected,omitempty"`
	YearlyCost  string   `json:"yearly_cost"`
	Alerts      []string `json:"alerts"`
	yearlyCents int64
}

// Result holds every finding. Totals count only active findings.
type Result struct {
	Findings     []Finding `json:"findings"`
	ActiveYearly string    `json:"active_yearly_total"`
	ActiveCount  int       `json:"active_count"`
	StatementEnd string    `json:"statement_end"`
	Warnings     []string  `json:"warnings"`
}

var (
	digitRuns   = regexp.MustCompile(`\S*\d\S*`)
	punctuation = regexp.MustCompile(`[^\p{L}\p{N}&' ]+`)
	noise       = map[string]bool{
		"POS": true, "DEBIT": true, "CREDIT": true, "CARD": true, "PURCHASE": true, "PAYMENT": true, "PMT": true, "PAYPAL": true, "SQ": true, "TST": true,
		"ACH": true, "RECURRING": true, "AUTOPAY": true, "AUTO": true, "PAY": true, "ONLINE": true, "WEB": true, "CHECKCARD": true, "VISA": true,
		"MASTERCARD": true, "TRANSFER": true, "DIRECT": true, "BILL": true, "SEPA": true, "COMPRA": true, "PAGAMENTO": true, "DEBITO": true, "CARTAO": true, "INT": true, "WWW": true, "COM": true, "THE": true,
	}
)

// MerchantName reduces a bank description to a stable name by dropping card
// words and anything containing a digit, such as dates and references.
func MerchantName(description string) string {
	text := strings.ToUpper(description)
	text = strings.NewReplacer("*", " ", ".", " ", "/", " ", "-", " ", "_", " ", "#", " ").Replace(text)
	text = digitRuns.ReplaceAllString(text, " ")
	text = punctuation.ReplaceAllString(text, " ")
	var kept []string
	for _, word := range strings.Fields(text) {
		if !noise[word] {
			kept = append(kept, word)
		}
		if len(kept) == 3 {
			break
		}
	}
	return strings.Join(kept, " ")
}

func formatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

func abs(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func median(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

// clusters splits a merchant's charges by amount, so two plans from one company
// count as two subscriptions rather than one irregular one.
func clusters(charges []Transaction) [][]Transaction {
	byAmount := append([]Transaction(nil), charges...)
	sort.SliceStable(byAmount, func(i, j int) bool { return abs(byAmount[i].Amount) < abs(byAmount[j].Amount) })
	var groups [][]Transaction
	for _, charge := range byAmount {
		if count := len(groups); count > 0 {
			// Compare with the smallest amount in the group, not the latest,
			// or amounts that drift upward would chain into one group.
			base := float64(abs(groups[count-1][0].Amount))
			if float64(abs(charge.Amount)) <= base*(1+sameAmountTolerance) {
				groups[count-1] = append(groups[count-1], charge)
				continue
			}
		}
		groups = append(groups, []Transaction{charge})
	}
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool { return group[i].Date.Before(group[j].Date) })
	}
	return groups
}

func days(a, b time.Time) float64 {
	return b.Sub(a).Hours() / 24
}

// removeSameCycleRepeats takes repeats within a few days out of the interval
// calculation and reports them, since a double charge shouldn't look like a
// change of billing cycle.
func removeSameCycleRepeats(group []Transaction) ([]Transaction, []string) {
	var kept []Transaction
	var alerts []string
	for _, charge := range group {
		if count := len(kept); count > 0 && days(kept[count-1].Date, charge.Date) <= duplicateWithinDays && charge.Amount == kept[count-1].Amount {
			alerts = append(alerts, fmt.Sprintf("possible double charge of %s on %s", formatCents(abs(charge.Amount)), charge.Date.Format("2006-01-02")))
			continue
		}
		kept = append(kept, charge)
	}
	return kept, alerts
}

func detect(merchant string, group []Transaction, end time.Time) (Finding, bool) {
	charges, alerts := removeSameCycleRepeats(group)
	if len(charges) < 2 {
		return Finding{}, false
	}
	intervals := make([]float64, 0, len(charges)-1)
	for i := 1; i < len(charges); i++ {
		intervals = append(intervals, days(charges[i-1].Date, charges[i].Date))
	}
	typical := median(intervals)
	var match *cadence
	for i := range cadences {
		if abs64(typical-cadences[i].days) <= cadences[i].tolerance {
			match = &cadences[i]
		}
	}
	if match == nil {
		return Finding{}, false
	}
	onTime := 0
	for _, interval := range intervals {
		if abs64(interval-match.days) <= match.tolerance {
			onTime++
		}
	}
	if float64(onTime)/float64(len(intervals)) < onTimeShare {
		return Finding{}, false
	}
	// Two charges only count for yearly billing, where two is all a statement
	// is likely to show. Anything more frequent needs three.
	if len(charges) < minLikelyCharges && match.name != "yearly" {
		return Finding{}, false
	}

	last := charges[len(charges)-1]
	finding := Finding{
		Merchant: merchant, Cadence: match.name, Confidence: Likely, Status: "active", Charges: len(group),
		LastAmount: formatCents(abs(last.Amount)), LastDate: last.Date.Format("2006-01-02"), FirstDate: charges[0].Date.Format("2006-01-02"),
		Alerts: alerts,
	}
	if finding.Alerts == nil {
		finding.Alerts = []string{}
	}
	if len(charges) >= minConfirmedCharges {
		finding.Confidence = Confirmed
	}
	changes := 0
	for i := 1; i < len(charges); i++ {
		if charges[i].Amount != charges[i-1].Amount {
			changes++
		}
	}
	// A subscription holds its price. A bill that changes most cycles, like
	// electricity, is recurring, but its changes aren't price increases.
	if changes*2 > len(charges)-1 {
		finding.Confidence = Likely
		finding.Alerts = append(finding.Alerts, "amount varies, so this may be a usage-based bill")
	} else {
		for i := 1; i < len(charges); i++ {
			before, after := abs(charges[i-1].Amount), abs(charges[i].Amount)
			if after > before {
				finding.Alerts = append(finding.Alerts, fmt.Sprintf("price went up from %s to %s on %s", formatCents(before), formatCents(after), charges[i].Date.Format("2006-01-02")))
			}
		}
	}
	finding.yearlyCents = int64(float64(abs(last.Amount))*match.perYear + 0.5)
	finding.YearlyCost = formatCents(finding.yearlyCents)
	if days(last.Date, end) > typical*1.5+float64(duplicateWithinDays) {
		finding.Status = "may have stopped"
	} else {
		finding.NextDate = last.Date.AddDate(0, 0, int(typical+0.5)).Format("2006-01-02")
	}
	return finding, true
}

func abs64(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

// Find looks for recurring money going out.
func Find(transactions []Transaction) Result {
	result := Result{Findings: []Finding{}, Warnings: []string{}, ActiveYearly: "0.00"}
	byMerchant := map[string][]Transaction{}
	var end time.Time
	for _, t := range transactions {
		if t.Date.After(end) {
			end = t.Date
		}
		if t.Amount < 0 {
			if merchant := MerchantName(t.Description); merchant != "" {
				byMerchant[merchant] = append(byMerchant[merchant], t)
			}
		}
	}
	if !end.IsZero() {
		result.StatementEnd = end.Format("2006-01-02")
	}
	merchants := make([]string, 0, len(byMerchant))
	for merchant := range byMerchant {
		merchants = append(merchants, merchant)
	}
	sort.Strings(merchants)
	var total int64
	for _, merchant := range merchants {
		for _, group := range clusters(byMerchant[merchant]) {
			if finding, ok := detect(merchant, group, end); ok {
				result.Findings = append(result.Findings, finding)
				if finding.Status == "active" {
					total += finding.yearlyCents
					result.ActiveCount++
				}
			}
		}
	}
	sort.SliceStable(result.Findings, func(i, j int) bool {
		a, b := result.Findings[i], result.Findings[j]
		if a.Status != b.Status {
			return a.Status == "active"
		}
		return a.yearlyCents > b.yearlyCents
	})
	result.ActiveYearly = formatCents(total)
	return result
}
