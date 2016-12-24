package main

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/mattn/go-sqlite3"
)

// Global variables
var db *sql.DB


const (
	dbContainerStatusRunning = 0
	dbContainerStatusDeleted = 1
	dbContainerStatusSuspended = 2
)


type containerDbInfo struct {
	ContainerName string
	ContainerBaseName string
	ContainerIP string
	ContainerUsername string
	ContainerPassword string
	ContainerExpiry int64
	ContainerStatus int64
}



// Check if a container exists for user userId and containerBaseName
func dbContainerExists(userId string, containerBaseName string) (bool, string, string) {
	var uuid string

	rows, err := dbQuery(db, "SELECT uuid, container_name FROM sessions WHERE userId=? AND containerBaseName=? AND status=?;", userId, containerBaseName, dbContainerStatusRunning)
	if err != nil {
		return false, "", ""
	}

	defer rows.Close()

	var doesExist bool

	doesExist = false
	var containerName string
	for rows.Next() {
		doesExist = true
		rows.Scan(&uuid, &containerName)
	}

	return doesExist, uuid, containerName
}


func dbGetContainerListForUser(userId string) (error, []containerDbInfo) {
	var containerList []containerDbInfo

	rows, err := dbQuery(db, "SELECT container_name, containerBaseName, container_ip, container_username, container_password, container_expiry, status FROM sessions WHERE status=? AND userId=?;", dbContainerStatusRunning, userId)
	if err != nil {
		logger.Errorf("dbquery error")
		return err, nil
	}
	defer rows.Close()

	for rows.Next() {
		var container containerDbInfo

		rows.Scan(
			&container.ContainerName,
			&container.ContainerBaseName,
			&container.ContainerIP,
			&container.ContainerUsername,
			&container.ContainerPassword,
			&container.ContainerExpiry,
			&container.ContainerStatus,
		)

		containerList = append(containerList, container)
	}

	return nil, containerList
}


func dbGetContainerForUser(userId string, containerBaseName string) (containerDbInfo, bool) {
	var container containerDbInfo;

	var sqlquery = "SELECT container_name, container_ip, container_username, container_password, container_expiry, status, containerBaseName"
	sqlquery += " FROM sessions WHERE status=? AND userId=? AND containerBaseName=?;"
	rows, err := dbQuery(db, sqlquery, dbContainerStatusRunning, userId, containerBaseName)
	if err != nil {
		return containerDbInfo{}, false
	}
	defer rows.Close()

	for rows.Next() {
		rows.Scan(
			&container.ContainerName,
			&container.ContainerIP,
			&container.ContainerUsername,
			&container.ContainerPassword,
			&container.ContainerExpiry,
			&container.ContainerStatus,
			&container.ContainerBaseName,
		)
	}

	return container, true
}


func dbNewContainer(id string, userId string, containerBaseName string, containerName string, containerIP string, containerUsername string, containerPassword string, containerExpiry int64, requestDate int64, requestIP string) (int64, error) {
	res, err := db.Exec(`
INSERT INTO sessions (
	status,
	uuid,
	userId,
	containerBaseName,
	container_name,
	container_ip,
	container_username,
	container_password,
	container_expiry,
	request_date,
	request_ip,
	request_terms) VALUES (0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`, id, userId, containerBaseName, containerName, containerIP, containerUsername, containerPassword, containerExpiry, requestDate, requestIP, "")
	if err != nil {
		return 0, err
	}

	containerID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return containerID, nil
}


func dbUpdateContainerExpire(uuid string, expiryDate int64) error {
	_, err := db.Exec("UPDATE sessions SET container_expiry=? WHERE id=?;", expiryDate, uuid)
	return err
}


func dbExpire(id int64) error {
	_, err := db.Exec("UPDATE sessions SET status=? WHERE id=?;", dbContainerStatusDeleted, id)
	return err
}


func dbExpireUuid(uuid string) error {
	_, err := db.Exec("UPDATE sessions SET status=? WHERE uuid=?;", dbContainerStatusDeleted, uuid)
	return err
}


func dbActiveContainerCount() (int, error) {
	var count int

	statement := fmt.Sprintf("SELECT count(*) FROM sessions WHERE status=%d;", dbContainerStatusRunning)
	err := db.QueryRow(statement).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}


func dbActiveContainerCountForIP(ip string) (int, error) {
	var count int

	statement := `SELECT count(*) FROM sessions WHERE status=? AND request_ip=?;`
	err := db.QueryRow(statement, dbContainerStatusRunning, ip).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}


func dbNextExpire() (int, error) {
	var expire int

	statement := fmt.Sprintf("SELECT MIN(container_expiry) FROM sessions WHERE status=%d;", dbContainerStatusRunning)
	err := db.QueryRow(statement).Scan(&expire)
	if err != nil {
		return 0, err
	}

	return expire, nil
}


func dbIsLockedError(err error) bool {
	if err == nil {
		return false
	}
	if err == sqlite3.ErrLocked || err == sqlite3.ErrBusy {
		return true
	}
	if err.Error() == "database is locked" {
		return true
	}
	return false
}


func dbIsNoMatchError(err error) bool {
	if err == nil {
		return false
	}
	if err.Error() == "sql: no rows in result set" {
		return true
	}
	return false
}


func dbQueryRowScan(db *sql.DB, q string, args []interface{}, outargs []interface{}) error {
	for {
		err := db.QueryRow(q, args...).Scan(outargs...)
		if err == nil {
			return nil
		}
		if dbIsNoMatchError(err) {
			return err
		}
		if !dbIsLockedError(err) {
			return err
		}
		time.Sleep(1 * time.Second)
	}
}


func dbQuery(db *sql.DB, q string, args ...interface{}) (*sql.Rows, error) {
	for {
		result, err := db.Query(q, args...)
		if err == nil {
			return result, nil
		}
		if !dbIsLockedError(err) {
			return nil, err
		}
		time.Sleep(1 * time.Second)
	}
}


func dbDoQueryScan(db *sql.DB, q string, args []interface{}, outargs []interface{}) ([][]interface{}, error) {
	rows, err := db.Query(q, args...)
	if err != nil {
		return [][]interface{}{}, err
	}
	defer rows.Close()
	result := [][]interface{}{}
	for rows.Next() {
		ptrargs := make([]interface{}, len(outargs))
		for i := range outargs {
			switch t := outargs[i].(type) {
			case string:
				str := ""
				ptrargs[i] = &str
			case int:
				integer := 0
				ptrargs[i] = &integer
			default:
				return [][]interface{}{}, fmt.Errorf("Bad interface type: %s\n", t)
			}
		}
		err = rows.Scan(ptrargs...)
		if err != nil {
			return [][]interface{}{}, err
		}
		newargs := make([]interface{}, len(outargs))
		for i := range ptrargs {
			switch t := outargs[i].(type) {
			case string:
				newargs[i] = *ptrargs[i].(*string)
			case int:
				newargs[i] = *ptrargs[i].(*int)
			default:
				return [][]interface{}{}, fmt.Errorf("Bad interface type: %s\n", t)
			}
		}
		result = append(result, newargs)
	}
	err = rows.Err()
	if err != nil {
		return [][]interface{}{}, err
	}
	return result, nil
}


func dbQueryScan(db *sql.DB, q string, inargs []interface{}, outfmt []interface{}) ([][]interface{}, error) {
	for {
		result, err := dbDoQueryScan(db, q, inargs, outfmt)
		if err == nil {
			return result, nil
		}
		if !dbIsLockedError(err) {
			return nil, err
		}
		time.Sleep(1 * time.Second)
	}
}


func dbSetup() error {
	var err error

	db, err = sql.Open("sqlite3", fmt.Sprintf("lxd-demo.sqlite3?_busy_timeout=5000&_txlock=exclusive"))
	if err != nil {
		return err
	}

	err = dbCreateTables()
	if err != nil {
		return err
	}

	return nil
}


func dbCreateTables() error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
		userId VARCHAR(64) NOT NULL,
		containerBaseName VARCHAR(64) NOT NULL,
    uuid VARCHAR(36) NOT NULL,
    status INTEGER NOT NULL,
    container_name VARCHAR(64) NOT NULL,
    container_ip VARCHAR(39) NOT NULL,
    container_username VARCHAR(10) NOT NULL,
    container_password VARCHAR(10) NOT NULL,
    container_expiry INT NOT NULL,
    request_date INT NOT NULL,
    request_ip VARCHAR(39) NOT NULL,
    request_terms VARCHAR(64) NOT NULL
);
`)
	if err != nil {
		return err
	}

	return nil
}


func dbActiveContainer() ([][]interface{}, error) {
	q := fmt.Sprintf("SELECT id, container_name, container_expiry FROM sessions WHERE status=%d;", dbContainerStatusRunning)
	var containerID int
	var containerName string
	var containerExpiry int
	outfmt := []interface{}{containerID, containerName, containerExpiry}
	result, err := dbQueryScan(db, q, nil, outfmt)
	if err != nil {
		return nil, err
	}

	return result, nil
}
