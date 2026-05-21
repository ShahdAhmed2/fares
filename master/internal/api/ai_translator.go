package api

import (
	"fmt"
	"regexp"
	"strings"
)

// AITranslator converts natural language to SQL
type AITranslator struct{}

// TranslationResult holds the result of NL-to-SQL conversion
type TranslationResult struct {
	Input    string `json:"input"`
	SQL      string `json:"sql"`
	Confidence float64 `json:"confidence"`
	Explanation string `json:"explanation"`
}

// rule maps a pattern to a SQL builder
type rule struct {
	pattern     *regexp.Regexp
	builder     func(matches []string) (string, string, float64)
}

var rules []rule

func init() {
	rules = []rule{
		// "show/get/list all clients/customers from <city>"
		{
			regexp.MustCompile(`(?i)(show|get|list|find)\s+(all\s+)?(clients?|customers?|people|users?|records?)\s+(from|in|of)\s+(\w+)`),
			func(m []string) (string, string, float64) {
				city := strings.Title(strings.ToLower(m[5]))
				return fmt.Sprintf("SELECT * FROM client WHERE city = '%s';", city),
					fmt.Sprintf("Fetching all clients from %s", city), 0.92
			},
		},
		// "show/count how many clients in <city>"
		{
			regexp.MustCompile(`(?i)(count|how many)\s+(clients?|customers?|records?|people|users?)\s+(in|from|at)\s+(\w+)`),
			func(m []string) (string, string, float64) {
				city := strings.Title(strings.ToLower(m[4]))
				return fmt.Sprintf("SELECT COUNT(*) as total FROM client WHERE city = '%s';", city),
					fmt.Sprintf("Counting clients in %s", city), 0.95
			},
		},
		// "insert/add a client named <name> with balance <amount>"
		{
			regexp.MustCompile(`(?i)(insert|add|create)\s+(a\s+)?(client|customer|person|user)\s+named\s+(\w+\s+\w+|\w+)\s+with\s+balance\s+([\d.]+)`),
			func(m []string) (string, string, float64) {
				name := strings.Title(strings.ToLower(m[4]))
				balance := m[5]
				return fmt.Sprintf(`INSERT INTO client(name,national_id,phone,email,gender,birth_date,city,address,account_type,balance,created_at) VALUES('%s','00000000000000','01000000000','no@email.com','Male','1990-01-01','Cairo','N/A','Savings',%s,date('now'));`, name, balance),
					fmt.Sprintf("Adding new client %s with balance %s", name, balance), 0.88
			},
		},
		// "show clients with balance greater/more than <amount>"
		{
			regexp.MustCompile(`(?i)(show|get|list|find)\s+(clients?|customers?)\s+with\s+balance\s+(greater|more|above|over)\s+than\s+([\d.]+)`),
			func(m []string) (string, string, float64) {
				amount := m[4]
				return fmt.Sprintf("SELECT id, name, city, balance FROM client WHERE balance > %s ORDER BY balance DESC;", amount),
					fmt.Sprintf("Finding clients with balance > %s", amount), 0.93
			},
		},
		// "show clients with balance less than <amount>"
		{
			regexp.MustCompile(`(?i)(show|get|list|find)\s+(clients?|customers?)\s+with\s+balance\s+(less|below|under)\s+than\s+([\d.]+)`),
			func(m []string) (string, string, float64) {
				amount := m[4]
				return fmt.Sprintf("SELECT id, name, city, balance FROM client WHERE balance < %s ORDER BY balance ASC;", amount),
					fmt.Sprintf("Finding clients with balance < %s", amount), 0.93
			},
		},
		// "show male/female clients"
		{
			regexp.MustCompile(`(?i)(show|list|get|find)\s+(all\s+)?(male|female)\s+(clients?|customers?|users?)`),
			func(m []string) (string, string, float64) {
				gender := strings.Title(strings.ToLower(m[3]))
				return fmt.Sprintf("SELECT id, name, city, gender, balance FROM client WHERE gender = '%s';", gender),
					fmt.Sprintf("Fetching all %s clients", gender), 0.94
			},
		},
		// "show savings/current/business/fixed deposit accounts"
		{
			regexp.MustCompile(`(?i)(show|list|get)\s+(all\s+)?(savings|current|business|fixed deposit)\s+(accounts?|clients?|customers?)`),
			func(m []string) (string, string, float64) {
				atype := strings.Title(strings.ToLower(m[3]))
				if strings.ToLower(m[3]) == "fixed" {
					atype = "Fixed Deposit"
				}
				return fmt.Sprintf("SELECT id, name, city, account_type, balance FROM client WHERE account_type = '%s';", atype),
					fmt.Sprintf("Listing %s account holders", atype), 0.91
			},
		},
		// "total balance in <city>"
		{
			regexp.MustCompile(`(?i)(total|sum|aggregate)\s+(balance|money|funds?)\s+(in|at|for|from)\s+(\w+)`),
			func(m []string) (string, string, float64) {
				city := strings.Title(strings.ToLower(m[4]))
				return fmt.Sprintf("SELECT city, SUM(balance) as total_balance, COUNT(*) as client_count FROM client WHERE city = '%s' GROUP BY city;", city),
					fmt.Sprintf("Calculating total balance for %s", city), 0.90
			},
		},
		// "delete client with id <n>"
		{
			regexp.MustCompile(`(?i)(delete|remove)\s+(client|customer|record)\s+(with\s+)?(id|number)\s+(\d+)`),
			func(m []string) (string, string, float64) {
				id := m[5]
				return fmt.Sprintf("DELETE FROM client WHERE id = %s;", id),
					fmt.Sprintf("Deleting client with ID %s", id), 0.97
			},
		},
		// "update balance of client <id> to <amount>"
		{
			regexp.MustCompile(`(?i)(update|set|change)\s+(balance\s+of\s+)?(client|customer)\s+(\d+)\s+(to|with)\s+([\d.]+)`),
			func(m []string) (string, string, float64) {
				id := m[4]
				amount := m[6]
				return fmt.Sprintf("UPDATE client SET balance = %s WHERE id = %s;", amount, id),
					fmt.Sprintf("Updating balance of client ID %s to %s", id, amount), 0.96
			},
		},
		// "show all clients" fallback
		{
			regexp.MustCompile(`(?i)(show|list|get|display)\s+(all\s+)?(clients?|customers?|records?|users?)`),
			func(m []string) (string, string, float64) {
				return "SELECT id, name, city, gender, account_type, balance FROM client LIMIT 100;",
					"Listing all clients (first 100)", 0.85
			},
		},
		// "average balance by city"
		{
			regexp.MustCompile(`(?i)(average|avg|mean)\s+balance\s+(by\s+)?(city|governorate|location)`),
			func(m []string) (string, string, float64) {
				return "SELECT city, AVG(balance) as avg_balance, COUNT(*) as clients FROM client GROUP BY city ORDER BY avg_balance DESC;",
					"Calculating average balance per city", 0.93
			},
		},
		// "top <n> richest clients"
		{
			regexp.MustCompile(`(?i)top\s+(\d+)\s+(richest|wealthiest|highest)\s+(clients?|customers?|accounts?)`),
			func(m []string) (string, string, float64) {
				n := m[1]
				return fmt.Sprintf("SELECT id, name, city, account_type, balance FROM client ORDER BY balance DESC LIMIT %s;", n),
					fmt.Sprintf("Finding top %s wealthiest clients", n), 0.96
			},
		},
	}
}

// Translate converts natural language to SQL
func (t *AITranslator) Translate(input string) TranslationResult {
	input = strings.TrimSpace(input)

	for _, r := range rules {
		if m := r.pattern.FindStringSubmatch(input); m != nil {
			sql, explanation, confidence := r.builder(m)
			return TranslationResult{
				Input:       input,
				SQL:         sql,
				Confidence:  confidence,
				Explanation: explanation,
			}
		}
	}

	// Unknown — return a helpful message
	return TranslationResult{
		Input:       input,
		SQL:         "",
		Confidence:  0,
		Explanation: "Could not parse this query. Try phrases like: 'show all clients from Cairo', 'count customers in Alexandria', 'top 10 richest clients'",
	}
}
