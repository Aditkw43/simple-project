package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

type Config struct {
	Router   *mux.Router
	Database *sql.DB
}

type Todo struct {
	ID          int    `json:"id,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	IsDone      bool   `json:"is_done"`
}

type Response struct {
	Data    interface{} `json:"data"`
	Status  int         `json:"status"`
	Message string      `json:"message"`
}

const (
	MESSAGE_SUCCESS = "Success"
	MESSAGE_FAILED  = "Failed"
)

func buildResponse(w http.ResponseWriter, data interface{}, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	result := Response{
		Data:    data,
		Status:  status,
		Message: message,
	}
	json.NewEncoder(w).Encode(result)
}

func setupDatabase() *sql.DB {
	// Get data from .env
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	// Connection to database postgresql local
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	migrations(connStr, db)
	return db
}

func migrations(connStr string, db *sql.DB) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("Failed to get current directory")
	}

	dir := filepath.Dir(filename)
	sourceURL := "file://" + filepath.Join(dir, "schema")
	fmt.Println("source URL: ", sourceURL)

	// Migrasi database
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatal(err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		sourceURL,
		"postgres",
		driver,
	)

	if err != nil {
		log.Fatal(err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal(err)
	}
	log.Println("Migrations applied successfully...")
}

func (r *Config) Handler() {
	// Get all to-do list
	r.Router.HandleFunc(`/todo`, r.getTodos).Methods("GET")

	// Get detail to-do list
	r.Router.HandleFunc(`/todo/{id}`, r.getTodo).Methods("GET")

	// Add to-do list
	r.Router.HandleFunc(`/todo`, r.addTodo).Methods("POST")

	// Update to-do list
	r.Router.HandleFunc(`/todo/{id}`, r.updateTodo).Methods("PUT")

	// Remove to-do list
	r.Router.HandleFunc(`/todo/{id}`, r.deleteTodo).Methods("DELETE")

	fmt.Println("Server listening on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", r.Router))
}

func (conf *Config) getTodos(w http.ResponseWriter, r *http.Request) {
	var todos []Todo
	rows, err := conf.Database.Query("SELECT id, title, is_done FROM todo")
	if err != nil {
		buildResponse(w, todos, http.StatusInternalServerError, MESSAGE_FAILED)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var todo Todo
		err := rows.Scan(&todo.ID, &todo.Title, &todo.IsDone)
		if err != nil {
			buildResponse(w, todos, http.StatusInternalServerError, MESSAGE_FAILED)
			return
		}
		todos = append(todos, todo)
	}

	buildResponse(w, todos, http.StatusOK, MESSAGE_SUCCESS)
}

func (conf *Config) getTodo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	todoID := vars["id"]

	var todo Todo
	if err := conf.Database.QueryRow("SELECT title, description, is_done FROM todo WHERE id=$1", todoID).Scan(&todo.Title, &todo.Description, &todo.IsDone); err == sql.ErrNoRows {
		buildResponse(w, nil, http.StatusNotFound, MESSAGE_FAILED)
		return
	} else if err != nil {
		buildResponse(w, nil, http.StatusInternalServerError, MESSAGE_FAILED)
		return
	}

	buildResponse(w, todo, http.StatusOK, MESSAGE_SUCCESS)
}

func (conf *Config) addTodo(w http.ResponseWriter, r *http.Request) {
	var newTodo Todo
	json.NewDecoder(r.Body).Decode(&newTodo)

	if _, err := conf.Database.Exec("INSERT INTO todo(title, description) VALUES($1,$2)", newTodo.Title, newTodo.Description); err != nil {
		buildResponse(w, newTodo, http.StatusInternalServerError, MESSAGE_FAILED)
		return
	}

	buildResponse(w, newTodo, http.StatusCreated, MESSAGE_SUCCESS)
}

func (conf *Config) updateTodo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	todoID := vars["id"]

	var (
		updatedTodo, existingTodo Todo
		err                       error
	)
	json.NewDecoder(r.Body).Decode(&updatedTodo)

	if err = conf.Database.QueryRow("SELECT id FROM todo WHERE id=$1", todoID).Scan(existingTodo.ID); err == sql.ErrNoRows {
		fmt.Println(err)
		buildResponse(w, nil, http.StatusNotFound, MESSAGE_FAILED)
		return
	} else if err != nil {
		fmt.Println(err)
		buildResponse(w, nil, http.StatusInternalServerError, MESSAGE_FAILED)
		return
	}
	fmt.Println("ERROR: ", err)
	fmt.Println("TODO ID:", todoID)

	if _, err = conf.Database.Exec("UPDATE todo SET title = $2, description = $3, is_done = $4 WHERE id = $1", todoID, updatedTodo.Title, updatedTodo.Description, updatedTodo.IsDone); err != nil {
		buildResponse(w, updatedTodo, http.StatusInternalServerError, MESSAGE_FAILED)
		return
	}

	buildResponse(w, updatedTodo, http.StatusOK, MESSAGE_SUCCESS)
}

func (conf *Config) deleteTodo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	todoID := vars["id"]
	var deletedTodo Todo

	if err := conf.Database.QueryRow("SELECT title, description FROM todo WHERE id=$1", todoID).Scan(&deletedTodo.Title, &deletedTodo.Description); err == sql.ErrNoRows {
		buildResponse(w, nil, http.StatusNotFound, MESSAGE_FAILED)
		return
	} else if err != nil {
		buildResponse(w, nil, http.StatusInternalServerError, MESSAGE_FAILED)
		return
	}

	if _, err := conf.Database.Exec("DELETE FROM todo WHERE id = $1", todoID); err != nil {
		buildResponse(w, nil, http.StatusInternalServerError, MESSAGE_FAILED)
		return
	}

	buildResponse(w, deletedTodo, http.StatusOK, MESSAGE_SUCCESS)
}

func main() {
	config := &Config{
		Router:   mux.NewRouter(),
		Database: setupDatabase(),
	}
	defer config.Database.Close()
	config.Handler()
}
