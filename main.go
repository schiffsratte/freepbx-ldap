package main

import (
	"bufio"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/nyaruka/phonenumbers/v2"
	"github.com/vjeantet/goldap/message"
	ldap "github.com/vjeantet/ldapserver"
)

var debugEnabled bool

func main() {
	sqlserver, sqluser, sqlpass, sqldb, ldapport, debug := getCreds()
	debugEnabled = debug

	log.Printf("DB Connection: Server=%s User=%s DB=%s", sqlserver, sqluser, sqldb)

	err := SQLConnect(sqlserver, sqluser, sqlpass, sqldb)
	if err != nil {
		log.Printf("DB ERROR: %s", err.Error())
		return
	}

	//Create a new LDAP Server
	server := ldap.NewServer()

	//Create routes bindings
	routes := ldap.NewRouteMux()

	routes.Bind(handleBind)
	routes.Search(handleSearchDSE).Label("Search - Generic")

	//Attach routes to server
	server.Handle(routes)

	// listen on 10389 and serve
	go server.ListenAndServe(":" + ldapport)

	// When CTRL+C, SIGINT and SIGTERM signal occurs
	// Then stop server gracefully
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	close(ch)
	server.Stop()
}

func getEnvVar(key, fallback string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = fallback
	}
	return value
}

func getCreds() (string, string, string, string, string, bool) {
	config, err := loadConfig("/opt/freepbx-ldap/freepbx-ldap.conf")
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("Cannot load configuration: %v", err)
	}

	sqlServer := getConfig(config, "FREEPBX_SQLSERVER", "localhost:3306")
	sqlUser := getConfig(config, "FREEPBX_SQLUSER", "")
	sqlPassword := getConfig(config, "FREEPBX_SQLPASSWORD", "")
	sqlDatabase := getConfig(config, "FREEPBX_SQLDATABASE", "asterisk")
	ldapPort := getConfig(config, "LDAP_PORT", "10389")
	debug := parseBool(getConfig(config, "DEBUG", "OFF"))
	return sqlServer, sqlUser, sqlPassword, sqlDatabase, ldapPort, debug
}

func handleBind(w ldap.ResponseWriter, m *ldap.Message) {
	res := ldap.NewBindResponse(ldap.LDAPResultSuccess)
	w.Write(res)
}

func handleSearchDSE(w ldap.ResponseWriter, m *ldap.Message) {
	r := m.GetSearchRequest()

	res := ldap.NewSearchResultDoneResponse(ldap.LDAPResultSuccess)
	defer w.Write(res)

	debugf("Request BaseDn=%s", r.BaseObject())
	debugf("Request Filter=%#v", r.Filter())
	debugf("Request FilterString=%s", r.FilterString())
	debugf("Request Attributes=%s", r.Attributes())
	debugf("Request TimeLimit=%d", r.TimeLimit().Int())
	debugf("Request SizeLimit=%d", r.SizeLimit().Int())

	//sql := "SELECT name, extension FROM visual_phonebook"
	sql := "SELECT CONCAT(TRIM(lastname) ,' ', TRIM(firstname)) as name, number FROM visual_phonebook INNER JOIN visual_phonebook_phones ON visual_phonebook.id = visual_phonebook_phones.contact_id"

	sqlVals := []interface{}{}

	swapField := func(v string) string {
		switch v {
		case "displayName":
			return "CONCAT(TRIM(lastname), ' ', TRIM(firstname))"

		case "telephoneNumber":
			// SQL-seitig alle üblichen Formatierungszeichen entfernen.
			return `
        	REPLACE(
            	    REPLACE(
                	REPLACE(
                    	    REPLACE(
                        	REPLACE(
                        	    REPLACE(number, ' ', ''),
                    	        '-', ''),
                	    '.', ''),
            	        '/', ''),
            	'(', ''),
        	')', '')
    	    `

		default:
			debugf("Invalid Field name (%s), returned displayName", v)
			return "CONCAT(TRIM(lastname), ' ', TRIM(firstname))"
		}
	}

	getSubstringSearch := func(v []message.Substring) string {

		var isnumeric = false
		for _, fs := range v {
			switch fsv := fs.(type) {
			case message.SubstringInitial:
				isnumeric = isNumeric(string(fsv))
				if !isnumeric {
					return string(fsv) + "%"
				} else {
					return string(fsv) + "%"
				}
			case message.SubstringAny:
				isnumeric = isNumeric(string(fsv))
				if !isnumeric {
					return "%" + string(fsv) + "%"
				} else {
					return "%" + string(fsv) + "%"
				}

			case message.SubstringFinal:
				isnumeric = isNumeric(string(fsv))
				if !isnumeric {
					return "%" + string(fsv)
				} else {
					return "%" + string(fsv)
				}

			}
		}
		return ""
	}

	var recursiveFilter func(filter interface{}, root bool) string
	recursiveFilter = func(filter interface{}, root bool) string {
		where := ""

		var filterProcessSub func(vsub interface{}) string
		filterProcessSub = func(vsub interface{}) string {
			switch vs := vsub.(type) {
			case message.FilterGreaterOrEqual:
				sqlVals = append(sqlVals, vs.AssertionValue())
				return swapField(string(vs.AttributeDesc())) + " >= ?"
			case message.FilterLessOrEqual:
				sqlVals = append(sqlVals, vs.AssertionValue())
				return swapField(string(vs.AttributeDesc())) + " <= ?"
			case message.FilterEqualityMatch:
				sqlVals = append(sqlVals, vs.AssertionValue())
				return swapField(string(vs.AttributeDesc())) + " = ?"
			case message.FilterSubstrings:
				searchValue := getSubstringSearch(vs.Substrings())
				rawValue := strings.Trim(searchValue, "%")

				if isNumericSearch(rawValue) {
					variants := normalizePhoneSearch(rawValue)
					if len(variants) == 0 {
						return ""
					}

					phoneField := swapField("telephoneNumber")
					conditions := make([]string, 0, len(variants))

					for _, variant := range variants {
						conditions = append(conditions, phoneField+" LIKE ?")
						sqlVals = append(sqlVals, "%"+variant+"%")
					}

					return "( " + strings.Join(conditions, " OR ") + " )"
				}

				// normale Namenssuche
				sqlVals = append(sqlVals, "%"+rawValue+"%")
				return swapField("displayName") + " LIKE ?"
			case message.FilterAnd:
				return recursiveFilter(vs, false)
			case message.FilterOr:
				return recursiveFilter(vs, false)
			case message.FilterNot:
				return " NOT ( " + filterProcessSub(vs.Filter) + " ) "
			default:
				return ""
			}

		}

		switch val := filter.(type) {
		case message.FilterAnd:
			i := 0
			for _, vsub := range val {
				addWhere := func() {
					if i > 0 {
						where += " AND "
					}
					i++
				}
				if ret := filterProcessSub(vsub); ret != "" {
					addWhere()
					where += ret
				}
			}
		case message.FilterOr:
			i := 0
			for _, vsub := range val {
				addWhere := func() {
					if i > 0 {
						where += " OR "
					}
					i++
				}
				if ret := filterProcessSub(vsub); ret != "" {
					addWhere()
					where += ret
				}
			}
		case message.FilterSubstrings:
			if ret := filterProcessSub(val); ret != "" {
				where += ret
			}
		default:
			debugf("Searching without filter...")
		}

		if where != "" {
			if root {
				where = " WHERE " + where
			} else {
				where = " ( " + where + " ) "
			}
		}

		return where
	}

	sql += " " + recursiveFilter(r.Filter(), true) + " "

	sql += " ORDER BY lastname ASC LIMIT 0, ?"
	if r.SizeLimit().Int() > 0 {
		sqlVals = append(sqlVals, r.SizeLimit().Int())
	} else {
		sqlVals = append(sqlVals, 99)
	}

	debugf("Query SQL: %s %#v", sql, sqlVals)
	result, err := SQLSearch(sql, sqlVals)
	if err != nil {
		debugf("SQL ERROR: %s", err)
	}
	debugf("resultat %#v", result)

	for _, entry := range result {
		e := ldap.NewSearchResultEntry("")
		e.AddAttribute("displayName", message.AttributeValue(entry.Name))
		e.AddAttribute("telephoneNumber", message.AttributeValue(entry.Number))
		debugf("Number=%s", entry.Number)
		debugf("Name=%s", entry.Name)
		//#debugf("Number=%#v", entry)
		w.Write(e)
	}
}

// create a function to check if string is numeric
func isNumeric(word string) bool {
	return regexp.MustCompile(`\d`).MatchString(word)
	// calling regexp.MustCompile() function to create the regular expression.
	// calling MatchString() function that returns a bool that
	// indicates whether a pattern is matched by the string.
}
func isNumericSearch(s string) bool {
	// Telefonnummernsuche erkennen:
	// Ziffern sowie typische Telefonzeichen sind erlaubt.
	s = strings.TrimSpace(s)

	if s == "" {
		return false
	}

	matched, _ := regexp.MatchString(`^[0-9+(). /-]+$`, s)
	return matched
}

func digitsOnly(s string) string {
	re := regexp.MustCompile(`\D`)
	return re.ReplaceAllString(s, "")
}

func normalizePhoneSearch(input string) []string {
	input = strings.TrimSpace(input)

	if input == "" {
		return nil
	}

	variants := make([]string, 0)

	add := func(v string) {
		v = digitsOnly(v)

		if v == "" {
			return
		}

		for _, existing := range variants {
			if existing == v {
				return
			}
		}

		variants = append(variants, v)
	}

	// Immer auch das verwenden, was der Benutzer tatsächlich eingegeben hat.
	//
	// 0xx
	// +33xx
	// 0033x
	// usw.
	add(input)

	rawDigits := digitsOnly(input)

	if strings.HasPrefix(rawDigits, "0") && len(rawDigits) > 1 {
		add(strings.TrimPrefix(rawDigits, "0"))
	}

	// 00... in +... umwandeln, damit libphonenumber es eindeutig
	// als internationale Nummer erkennt.
	parseInput := input
	if strings.HasPrefix(parseInput, "00") {
		parseInput = "+" + strings.TrimPrefix(parseInput, "00")
	}

	num, err := phonenumbers.Parse(parseInput, "FR")
	if err != nil {
		// Bei Teilnummern wie "076" ist Parse erwartungsgemäß nicht
		// immer möglich. Der rohe Suchwert bleibt trotzdem erhalten.
		return variants
	}

	// Internationale E.164-Darstellung:
	//
	// +112233445566
	//
	// digitsOnly -> 112233445566
	add(phonenumbers.Format(num, phonenumbers.E164))

	// Nationale Darstellung:
	//
	// 01 22 33 44 55 66
	//
	// digitsOnly -> 01122334455667
	add(phonenumbers.Format(num, phonenumbers.NATIONAL))

	// Internationale menschenlesbare Darstellung:
	//
	// + 11 22 33 44 55 66
	add(phonenumbers.Format(num, phonenumbers.INTERNATIONAL))

	return variants
}

func getConfig(config map[string]string, key string, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	if value, ok := config[key]; ok && value != "" {
		return value
	}

	return defaultValue
}
func loadConfig(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := make(map[string]string)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Leerzeilen und Kommentare ignorieren
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		config[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	return config, scanner.Err()
}
func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func debugf(format string, args ...interface{}) {
	if debugEnabled {
		log.Printf(format, args...)
	}
}
