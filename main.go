package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/vjeantet/goldap/message"
	ldap "github.com/vjeantet/ldapserver"
)

func main() {
	sqlserver, sqluser, sqlpass, sqldb := getCreds()
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
	go server.ListenAndServe(":10389")

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

func getCreds() (string, string, string, string) {
	sqlserver := getEnvVar("FREEPBX_SQLSERVER", "localhost:3306")
	sqluser := getEnvVar("FREEPBX_SQLUSER", "myuserorroot")
	sqlpass := getEnvVar("FREEPBX_SQLPASS", "mypassword")
	sqldb := getEnvVar("FREEPBX_SQLDB", "asterisk")
	return sqlserver, sqluser, sqlpass, sqldb
}

func handleBind(w ldap.ResponseWriter, m *ldap.Message) {
	res := ldap.NewBindResponse(ldap.LDAPResultSuccess)
	w.Write(res)
}

func handleSearchDSE(w ldap.ResponseWriter, m *ldap.Message) {
	r := m.GetSearchRequest()

	res := ldap.NewSearchResultDoneResponse(ldap.LDAPResultSuccess)
	defer w.Write(res)

	log.Printf("Request BaseDn=%s", r.BaseObject())
	log.Printf("Request Filter=%#v", r.Filter())
	log.Printf("Request FilterString=%s", r.FilterString())
	log.Printf("Request Attributes=%s", r.Attributes())
	log.Printf("Request TimeLimit=%d", r.TimeLimit().Int())
	log.Printf("Request SizeLimit=%d", r.SizeLimit().Int())

	//sql := "SELECT name, extension FROM visual_phonebook"
	sql := "SELECT CONCAT(TRIM(lastname) ,' ', TRIM(firstname)) as name, number FROM visual_phonebook INNER JOIN visual_phonebook_phones ON visual_phonebook.id = visual_phonebook_phones.contact_id"

	sqlVals := []interface{}{}

	swapField := func(v string) string {
		switch v {
		case "displayName":
			return "lastname"
		case "telephoneNumber":
			return "number"
		default:
			log.Printf("Invalid Field name (%s), returned lastname", v)
			return "lastname"
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
				sqlVals = append(sqlVals, getSubstringSearch(vs.Substrings()))
				return swapField(string(vs.Type_())) + " LIKE ?"
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
			log.Printf("Searching without filter...")
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

	log.Printf("Query SQL: %s %#v", sql, sqlVals)
	result, err := SQLSearch(sql, sqlVals)
	if err != nil {
		log.Printf("SQL ERROR: %s", err)
	}
	fmt.Println(fmt.Sprintf("resultat %#v", result))

	for _, entry := range result {
		e := ldap.NewSearchResultEntry("")
		e.AddAttribute("displayName", message.AttributeValue(entry.Name))
		e.AddAttribute("telephoneNumber", message.AttributeValue(entry.Number))
		log.Printf("Number=%s", entry.Number)
		log.Printf("Name=%s", entry.Name)
		//#log.Printf("Number=%#v", entry)
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
