package messages

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Messages struct {
	dB *sqlx.DB
	Instance string `db:"instance"`
	append *sqlx.NamedStmt
	getAll *sqlx.NamedStmt
}

type Row struct {
	Role string         `db:"role" json:"role"`
	Instance string     `db:"instance" json:"-"`
	Message   string    `db:"message" json:"content"`
	Timestamp DBTime    `db:"timestamp" json:"-"`
}

func New(DB *sqlx.DB, hierarchy string) (*Messages, error) {
	m := &Messages {
		dB: DB,
		Instance: fmt.Sprintf("%s%s/", hierarchy, uuid.New().String()),
	}

	if err := m.prepareDB(); err != nil {
		return nil,err

	}
	return m, nil
}

func (m *Messages) prepareDB() error {
	_, err := m.dB.Exec(`CREATE TABLE messages ( role TEXT, message TEXT, timestamp TEXT, instance TEXT);`)

   if err != nil {
	return fmt.Errorf("error creating messages database: %v", err)
   }
	
	append , err := m.dB.PrepareNamed("INSERT INTO messages (role, message, timestamp, instance) VALUES (:role, :message, :timestamp, :instance)")
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %v", err)
	}
	m.append = append

	getAll, err := m.dB.PrepareNamed("SELECT * FROM messages WHERE instance = :instance ORDER BY rowid ASC")
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %v", err)
	}
	m.getAll = getAll 

	return nil
}

func (m *Messages) Append(role string, msg string) error {
	row := Row{
		Message:   msg,
		Timestamp: DBTime{time.Now()},
		Instance: m.Instance,
		Role: role,
	}

	if _, err := m.append.Exec(row); err != nil {
		return fmt.Errorf("failed to execute statement: %v", err)
	}

	return nil
}

func (m *Messages) Get() ([]Row, error) {
	rows, err := m.getAll.Queryx(m)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %v", err)
	}
	defer rows.Close()

	var messages []Row = []Row{}

	for rows.Next() {
		var row Row
		if err := rows.StructScan(&row); err != nil {
			return nil, fmt.Errorf("failed to scan message row: %v", err)
		}
		messages = append(messages, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error while iterating over messages: %v", err)
	}

	return messages, nil
}

func (m *Messages) GetMarshalled() (string, error) {
	rows, err := m.Get()
	if err != nil {
		return "", err
	}
	ret, err := json.Marshal(rows)
	if err != nil {
		return "", err
	}
	return string(ret), nil
}

func (m *Messages) Clear() error {
	stmt, err := m.dB.Prepare("DELETE FROM messages WHERE instance LIKE ?")
	if err != nil {
		return fmt.Errorf("failed to prepare statement to clear messages: %v", err)
	}

	if _, err := stmt.Exec(fmt.Sprintf("%s%%",m.Instance)); err != nil {
		return fmt.Errorf("failed to execute statement: %v", err)
	}
	return nil
}

func (m *Messages) Reset() error {
	_, err := m.dB.Exec("DELETE FROM messages")
	if err != nil {
		return fmt.Errorf("failed to reset messages: %v", err)
	}

	_, err = m.dB.Exec("DELETE FROM sqlite_sequence WHERE name='messages';")
	if err != nil {
		return fmt.Errorf("failed to reset sequence number: %v", err)
	}
	return nil
}

type DBTime struct 
{
	T time.Time
}

func (t *DBTime) Scan(src interface{}) error {
    var value string 
    switch src.(type) {
        case string: value = src.(string)
        default: return errors.New("Invalid type for time")
    }
	var err error
    tmp, err := time.Parse(time.RFC3339, value)
	*t = DBTime{tmp}
    return err
}

func (t DBTime) Value() (driver.Value, error) {
    return driver.Value(t.T.Format(time.RFC3339)), nil
}
