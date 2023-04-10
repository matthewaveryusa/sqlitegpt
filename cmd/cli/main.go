package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"

	"github.com/matthewaveryusa/sqlchatgpt/internal/github"
	"github.com/matthewaveryusa/sqlchatgpt/internal/messages"
	sqlite3 "github.com/mattn/go-sqlite3"
	stdlib "github.com/multiprocessio/go-sqlite3-stdlib"
	openai "github.com/sashabaranov/go-openai"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	OPENAI_API_KEY := os.Getenv("OPENAI_API_KEY")
	client := openai.NewClient(OPENAI_API_KEY)
	sql.Register("sqlite3_ext",
		&sqlite3.SQLiteDriver{
			ConnectHook: (func(conn *sqlite3.SQLiteConn) error {
				err := stdlib.ConnectHook(conn)
				if err != nil {
					return err
				}

				conn.CreateModule("github", &github.Module{})

				err = conn.RegisterFunc("openai", func(messages string) string {
					m := []openai.ChatCompletionMessage{}
					err := json.Unmarshal([]byte(messages), &m)
					if err != nil {
						return "error invalid messages"
					}
					resp, err := client.CreateChatCompletion(
						context.Background(),
						openai.ChatCompletionRequest{
							Model:    openai.GPT4,
							Messages: m,
							Stop:     []string{SQLResultHeader},
						},
					)

					if err != nil {
						return fmt.Sprintf("ChatCompletion error: %v\n", err)
					}
					return resp.Choices[0].Message.Content
				}, true,
				)
				if err != nil {
					return err
				}
				return nil
			}),
		})

	db, err := sqlx.Open("sqlite3_ext", ":memory:")
	if err != nil {
		panic(err)
	}

	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("create virtual table repo using github(id /*github id*/, full_name /*repo name*/, description /*repo description*/, html_url /*url of the repo*/)")
	if err != nil {
		log.Fatal(err)
	}

	//{
	//	rows, err := db.Query("select id, full_name, description, html_url from repo")
	//	if err != nil {
	//		log.Fatal(err)
	//	}
	//	defer rows.Close()
	//	for rows.Next() {
	//		var id, fullName, description, htmlURL string
	//		rows.Scan(&id, &fullName, &description, &htmlURL)
	//		//fmt.Printf("%s: %s\n\t%s\n\t%s\n\n", id, fullName, description, htmlURL)
	//	}
	//}

	for _, q := range []string{"PRAGMA function_list", "PRAGMA pragma_list", "PRAGMA module_list", "PRAGMA table_list", "select * from sqlite_master", "select * from sqlite_schema"} {
		fmt.Println(q)
		rows, err := db.Queryx(q)
		if err != nil {
			log.Fatal(err)
		}
		defer rows.Close()
		cols, err := rows.Columns()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(cols)
		for rows.Next() {
			cols, err := rows.SliceScan()
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%v\n", cols)
		}
	}

	messagesTable, err := messages.New(db, "")
	if err != nil {
		panic(err)
	}
	msgs, err := messagesTable.Get()
	if err != nil {
		panic(err)
	}
	if len(msgs) == 0 {
		const prompt = `You are an assistant that can query a sqlite database directly. Use the following format:
Question: Question here
Thought: You should always think about what to do
SQLQuery: The sqlite SQL query to run 
SQLResult: the result of the SQL query
... (this Thought/SQLQuery/SQLResult can repeat N times)
Final Answer: The answer to the original input question

For the sqlite SQL query, first create a syntactically correct sqlite query to run, then look at the results of the query and return the answer to the question as an sqlite SQL query. Unless the user specifies in his question a specific number of examples he wishes to obtain, always limit your query to at most 10 results using the LIMIT clause. You can order the results by a relevant column to return the most interesting examples in the database.

Never query for all the columns from a specific table, only ask for a the few relevant columns given the question.

There's no need to apologize: Stick to the format.

Begin!`
		err = messagesTable.Append(openai.ChatMessageRoleSystem, prompt)
		if err != nil {
			panic(err)
		}
	}
	err = messagesTable.Append(openai.ChatMessageRoleUser, fmt.Sprintf("%s%s", QuestionHeader, os.Args[1]))
	stmt, err := db.Prepare("SELECT openai(?)")
	if err != nil {
		panic(err)
	}
	errCnt := 0
	for errCnt < 3 {
		msgs, err := messagesTable.GetMarshalled()
		if err != nil {
			panic(err)
		}

		r := stmt.QueryRow(string(msgs))
		var s string
		err = r.Scan(&s)
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s\n", s)
		err = messagesTable.Append(openai.ChatMessageRoleAssistant, s)
		if err != nil {
			panic(err)
		}
		out, err := parseResult(s)
		if err != nil {
			errCnt++
			fmt.Printf("%sinvalid response format: %v.\n", SQLResultHeader, err)
			err = messagesTable.Append(openai.ChatMessageRoleUser, fmt.Sprintf("%sinvalid response format.", SQLResultHeader))
			if err != nil {
				panic(err)
			}
			continue
		}
		if out.isFinal {
			//read line from stdin
			reader := bufio.NewReader(os.Stdin)
			fmt.Print(QuestionHeader)
			q, _ := reader.ReadString('\n')
			q = strings.Trim(q, " \t\r\n")
			if q == "exit" {
				break
			}
			err = messagesTable.Append(openai.ChatMessageRoleUser, fmt.Sprintf("%s%s", QuestionHeader, q))
			if err != nil {
				panic(err)
			}
			continue
		}
		
		rows, err := db.Queryx(out.Output)
		if err != nil {
			errCnt++
			fmt.Printf("%s%v", SQLResultHeader,  err)
			err = messagesTable.Append(openai.ChatMessageRoleUser, fmt.Sprintf("%s%v", SQLResultHeader, err))
			if err != nil {
				panic(err)
			}
			continue
		}
		errCnt = 0
		defer rows.Close()
		sb := strings.Builder{}
		rowCount := 0
		cols, err := rows.Columns()
		if err != nil {
			panic(err)
		}
		sb.WriteString(fmt.Sprintf("%scolumns: %v\n", SQLResultHeader, cols))
		for rows.Next() {
			rowCount++
			cols, err := rows.SliceScan()
			if err != nil {
				log.Fatal(err)
			}
			_, err = sb.WriteString(fmt.Sprintf("%v\n", cols))
			if err != nil {
				log.Fatal(err)
			}
		}
		fmt.Print(sb.String())
		
		err = messagesTable.Append(openai.ChatMessageRoleUser, sb.String())
		if err != nil {
		  panic(err)
		}
	}

}

const QuestionHeader = "Question: "
const ThoughtHeader = "Thought: "
const SQLQueryHeader = "SQLQuery: "
const SQLResultHeader = "SQLResult: "
const FinalAnswerHeader = "Final Answer: "

type parseOutput struct {
	Thought  string
	Output string
	isFinal  bool
}

func parseResult(s string) (parseOutput, error) {
	var ti, te, q int
	q = strings.Index(s, FinalAnswerHeader)
	if q != -1 {
		q += len(FinalAnswerHeader)
		return parseOutput{Thought: "", Output: strings.Trim(s[q:], " \t\n\r"), isFinal: true}, nil
	}
	ti = strings.Index(s, ThoughtHeader)
	if ti == -1 {
		return parseOutput{}, errors.New("no thought header found")
	} else {
		q = strings.Index(s, SQLQueryHeader)
		if q == -1 {
			return parseOutput{}, errors.New("no query header found")
		}
        te = q
		q += len(SQLQueryHeader)
	}
	thought := strings.Trim(s[ti+len(ThoughtHeader):te], " \t\n\r")
	output := strings.Trim(s[q:], " \t\n\r")
	return parseOutput{Thought: thought, Output: output, isFinal: false}, nil
}
