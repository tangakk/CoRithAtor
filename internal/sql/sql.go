package sql

import (
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

var main_db *sql.DB

type expression struct {
	Id         int64   //айди
	Expression string  //выражение
	Status     string  //статус
	Result     float64 //результат
	User       string  //пользователь с выражением
}

func Init() {

	db, err := sql.Open("sqlite3", "data.db")
	if err != nil {
		panic(err)
	}

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	main_db = db

	err = createTables()
	if err != nil {
		panic(err)
	}
}

func Close() {
	main_db.Close()
}

func createTables() error {
	const (
		usersTable = `
	CREATE TABLE IF NOT EXISTS users( 
		login TEXT,
		password TEXT
	);`
		expressionsTable = `
	CREATE TABLE IF NOT EXISTS expressions(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		expression TEXT,
		status TEXT DEFAULT "In queue",
		calculated INTEGER DEFAULT 0,
		result REAL DEFAULT 0,
		user_login TEXT
	);
	`
	)
	if _, err := main_db.Exec(usersTable); err != nil {
		return err
	}
	if _, err := main_db.Exec(expressionsTable); err != nil {
		return err
	}

	return nil
}

func RegisterUser(login string, password string) error {
	//cначала проверка регистрации
	q := "SELECT COUNT(*) FROM users WHERE login=$1"

	rows, err := main_db.Query(q, login)
	if err != nil {
		return err
	}
	var i int
	defer rows.Close()
	rows.Next()
	err = rows.Scan(&i)
	if err != nil {
		return err
	}
	if i != 0 {
		return errors.New("user already exists")
	}
	rows.Close()
	q = "INSERT INTO users (login, password) VALUES ($1, $2)"
	_, err = main_db.Exec(q, login, password)
	if err != nil {
		return err
	}
	return nil
}

func PasswordIsCorrect(login string, password string) (bool, error) {
	q := "SELECT password FROM users WHERE login=$1"
	rows, err := main_db.Query(q, login)
	if err != nil {
		return false, err
	}
	var real_password string
	defer rows.Close()
	rows.Next()
	err = rows.Scan(&real_password)
	fmt.Println(err, real_password, "HELLO")
	if err != nil {
		return false, err
	}
	fmt.Println(real_password, password)
	return password == real_password, nil
}

func IsUserExists(login string) (bool, error) {
	q := "SELECT COUNT(*) FROM users WHERE login=$1"

	rows, err := main_db.Query(q, login)
	if err != nil {
		return false, err
	}
	var i int
	defer rows.Close()
	rows.Next()
	err = rows.Scan(&i)
	if err != nil {
		return false, err
	}

	return i != 0, nil
}

func AddExpression(expression string, user string) (int64, error) {
	q := "INSERT INTO expressions (expression, user_login) VALUES ($1, $2)"
	res, err := main_db.Exec(q, expression, user)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func GetExpressions(login string) ([]expression, error) {
	q := "SELECT id, expression, status, result FROM expressions WHERE user_login=$1 ORDER BY id"
	res := make([]expression, 0)
	rows, err := main_db.Query(q, login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tmp expression
		err = rows.Scan(&tmp.Id, &tmp.Expression, &tmp.Status, &tmp.Result)
		if err != nil {
			return nil, err
		}
		tmp.User = login
		res = append(res, tmp)
	}
	return res, nil
}

func GetExpressionById(login string, id int) (expression, error) {
	q := "SELECT id, expression, status, result FROM expressions WHERE user_login=$1 AND id=$2 ORDER BY id"
	rows, err := main_db.Query(q, login, id)
	if err != nil {
		return expression{}, err
	}
	defer rows.Close()
	if rows.Next() {
		var tmp expression
		err = rows.Scan(&tmp.Id, &tmp.Expression, &tmp.Status, &tmp.Result)
		if err != nil {
			return expression{}, err
		}
		tmp.User = login
		return tmp, nil
	}
	return expression{}, errors.New("no expression with such id found")
}

func GetNotCountedExpressions() ([]expression, error) {
	q := "SELECT id, expression FROM expressions WHERE calculated=0 ORDER BY id"
	res := make([]expression, 0)
	rows, err := main_db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tmp expression
		err = rows.Scan(&tmp.Id, &tmp.Expression)
		if err != nil {
			return nil, err
		}
		res = append(res, tmp)
	}
	return res, nil
}

func WriteResult(id int, result float64, status string) error {
	q := "UPDATE expressions SET status=$1, result=$2, calculated=1 WHERE id=$3"
	_, err := main_db.Exec(q, status, result, id)
	if err != nil {
		return err
	}
	return nil
}
