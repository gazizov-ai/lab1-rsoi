package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Person struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Age     *int   `json:"age,omitempty"`
	Address string `json:"address,omitempty"`
	Work    string `json:"work,omitempty"`
}

type PersonIn struct {
	Name    string `json:"name"`
	Age     *int   `json:"age,omitempty"`
	Address string `json:"address,omitempty"`
	Work    string `json:"work,omitempty"`
}

func main() {
	dsn := os.Getenv("DATABASE_URL")

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatalf("cannot connect to db: %v", err)
	}

	if _, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS persons (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	age INTEGER,
	address TEXT,
	work TEXT
	)`); err != nil {
		log.Fatalf("auto-migrate failed: %v", err)
	}

	r := buildRouter(db)

	addr := ":8080"
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

func listPersons(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	rows, err := db.Query(`SELECT id, name, age, address, work FROM persons ORDER BY id`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	var res []Person
	for rows.Next() {
		var p Person
		var age sql.NullInt32
		if err := rows.Scan(&p.ID, &p.Name, &age, &p.Address, &p.Work); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if age.Valid {
			a := int(age.Int32)
			p.Age = &a
		}
		res = append(res, p)
	}
	writeJSON(w, 200, res)
}

func getPerson(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var p Person
	var age sql.NullInt32
	err := db.QueryRow(`SELECT id, name, age, address, work FROM persons WHERE ID=$1`, id).Scan(&p.ID, &p.Name, &age, &p.Address, &p.Work)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	if age.Valid {
		a := int(age.Int32)
		p.Age = &a
	}
	writeJSON(w, 200, p)
}

func createPerson(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var in PersonIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid json")
		return
	}
	if in.Name == "" {
		writeValidation(w, "name", "must not be empty")
		return
	}
	var id int
	err := db.QueryRow(
		`INSERT INTO persons (name, age, address, work) VALUES ($1,$2,$3,$4) RETURNING id`,
		in.Name, in.Age, in.Address, in.Work,
	).Scan(&id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	w.Header().Set("Location", "/api/v1/persons/"+strconv.Itoa(id))
	w.WriteHeader(http.StatusCreated)
}

func patchPerson(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in PersonIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid json")
		return
	}

	var cur Person
	var age sql.NullInt32
	err := db.QueryRow(`SELECT id, name, age, address, work FROM persons WHERE id=$1`, id).
		Scan(&cur.ID, &cur.Name, &age, &cur.Address, &cur.Work)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	if age.Valid {
		a := int(age.Int32)
		cur.Age = &a
	}

	if in.Name != "" {
		cur.Name = in.Name
	}
	if in.Age != nil {
		cur.Age = in.Age
	}
	if in.Address != "" {
		cur.Address = in.Address
	}
	if in.Work != "" {
		cur.Work = in.Work
	}

	_, err = db.Exec(
		`UPDATE persons SET name=$1, age=$2, address=$3, work=$4 WHERE id=$5`,
		cur.Name, cur.Age, cur.Address, cur.Work, id,
	)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	writeJSON(w, 200, cur)
}

func deletePerson(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	_, _ = db.Exec(`DELETE FROM persons WHERE id=$1`, id)
	w.WriteHeader((http.StatusNoContent))
}

func buildRouter(db *sql.DB) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1/persons", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) { listPersons(w, r, db) })
		r.Post("/", func(w http.ResponseWriter, r *http.Request) { createPerson(w, r, db) })
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) { getPerson(w, r, db) })
			r.Patch("/", func(w http.ResponseWriter, r *http.Request) { patchPerson(w, r, db) })
			r.Delete("/", func(w http.ResponseWriter, r *http.Request) { deletePerson(w, r, db) })
		})
	})

	return r
}

func parseID(w http.ResponseWriter, r *http.Request) (int, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid id")
		return 0, false
	}
	return id, true
}

type errorResp struct {
	Message string            `json:"message"`
	Errors  map[string]string `json:"errors,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResp{Message: msg})
}

func writeValidation(w http.ResponseWriter, field, msg string) {
	writeJSON(w, http.StatusBadRequest, errorResp{
		Message: "validation failed",
		Errors:  map[string]string{field: msg},
	})
}
