package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

var (
	dbConn *sql.DB
)

func SQLConnect(sqlserver string, sqluser string, sqlpass string, sqldb string) (err error) {

	// IPv6 address in brackets without port
	if strings.HasPrefix(sqlserver, "[") && strings.HasSuffix(sqlserver, "]") {
		sqlserver += ":3306"

		// IPv4/hostname without port
	} else if !strings.ContainsRune(sqlserver, ':') {
		sqlserver += ":3306"
	}

	dbConn, err = sql.Open(
		"mysql",
		sqluser+":"+sqlpass+"@tcp("+sqlserver+")/"+sqldb,
	)

	if err == nil {
		err = dbConn.Ping()
	}

	return
}

type PhonebookEntry struct {
	Name   string
	Number string
}

func SQLSearch(sqlQuery string, sqlVals []interface{}) ([]*PhonebookEntry, error) {
	var (
		rows   *sql.Rows
		err    error
		result []*PhonebookEntry = []*PhonebookEntry{}
	)
	rows, err = dbConn.Query(sqlQuery, sqlVals...)
	if err != nil {
		return nil, fmt.Errorf("Database Error: %s", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			name   string
			number string
		)
		err := rows.Scan(&name, &number)
		if err != nil {
			return nil, fmt.Errorf("Database Error: %s", err)
		}
		result = append(result, &PhonebookEntry{
			Name:   name,
			Number: number,
		})
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("database error: %s", err)
	}

	return result, nil
}
