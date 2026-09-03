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
	if strings.ContainsRune(sqlserver, ':') {
		dbConn, err = sql.Open("mysql", sqluser+":"+sqlpass+"@tcp("+sqlserver+")/"+sqldb)
	} else {
		dbConn, err = sql.Open("mysql", sqluser+":"+sqlpass+"@tcp("+sqlserver+":3306)/"+sqldb)
	}
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
