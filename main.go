package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"

	"github.com/BurntSushi/toml"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	_ "github.com/joho/godotenv/autoload"
	"github.com/justinas/alice"
	_ "github.com/mattn/go-sqlite3"
)

func logRequestMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Infof("Request: %s %s", r.Method, r.URL.String())
		h.ServeHTTP(w, r)
	})
}

type appError interface {
	error
	Status() int
}

type statusError struct {
	status int
	err    error
}

func (e statusError) Error() string {
	return e.err.Error()
}

func (e statusError) Status() int {
	return e.status
}

type env struct {
	db *sqlx.DB
}

type appHandler struct {
	*env
	H func(e *env, w http.ResponseWriter, r *http.Request) error
}

func (ah appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := ah.H(ah.env, w, r)
	if err != nil {
		log.Println(err.Error())
		writeError(w, r, err)
	}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var errStatus int
	var errMessage string

	switch e := err.(type) {
	case appError:
		errStatus, errMessage = e.Status(), e.Error()
	default:
		errStatus, errMessage = http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)
	}
	if true { // TODO: replace with content-type detections
		w.WriteHeader(errStatus)
		enc := json.NewEncoder(w)
		err := enc.Encode(struct {
			Error string `json:"error"`
		}{
			errMessage,
		})
		if err == nil {
			return // encoded error as json, so we're done
		}
		// Otherwise, drop through and fall back to text response
		log.Warnf("Unable to encode error JSON! %s", err.Error())
	}

	http.Error(w, errMessage, errStatus)
}

// Returns specific stream id if mux var exists, else returns all
func getStreamHandler(e *env, w http.ResponseWriter, r *http.Request) error {
	const streamSQL = `
		SELECT
			id, display_name, is_public, start_at, end_at, stream_name, key
		FROM
			streams
	`

	var err error
	vars := mux.Vars(r)

	if id, ok := vars["id"]; ok { // Specific id
		var s stream
		err := e.db.Get(&s, streamSQL+`WHERE id=?`, id)
		if err == sql.ErrNoRows {
			return statusError{
				404,
				err,
			}
		} else if err != nil {
			log.Errorf("Error querying for stream: %s", err.Error())
			return err
		}
		err = json.NewEncoder(w).Encode(&s)
	} else { // List all streams
		streams := make([]stream, 0)
		err := e.db.Select(&streams, streamSQL)
		if err != nil {
			log.Errorf("Error querying for stream(s): %s", err.Error())
			return err
		}
		err = json.NewEncoder(w).Encode(streams)
	}

	if err != nil {
		log.Errorf("Error encoding json: %s", err.Error())
		return err
	}
	return nil
}

func deleteStreamHandler(e *env, w http.ResponseWriter, r *http.Request) error {
	const deleteSQL = `
		DELETE FROM streams WHERE id=?
	`

	id, ok := mux.Vars(r)["id"]
	if !ok { // No id in URL. Shouldn't happen as mux does matching
		return statusError{
			400,
			errors.New("No id in URL"),
		}
	}

	result, err := e.db.Exec(deleteSQL, id)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		log.Warnf("Unable to get number of rows affected: %s", err)
		return nil // Delete still happened though, so don't error
	}

	if n == 0 {
		http.Error(w, "Not found", http.StatusNotFound)
	}

	return nil
}

func createStreamHandler(e *env, w http.ResponseWriter, r *http.Request) error {
	tx, err := e.db.Begin()
	if err != nil {
		return err
	}

	var s stream

	dec := json.NewDecoder(r.Body)

	err = dec.Decode(&s)
	if err != nil {
		log.Debugf("Error decoding JSON: %s", err.Error())
		return statusError{
			400,
			err,
		}
	}

	if s.Key != "" { // Client not allowed to specify key

		return statusError{
			400,
			errors.New("Client-specified key not allowed"),
		}
	}

	s.Key = randomString(20)

	result, err := tx.Exec(`
		INSERT INTO streams (
			display_name, is_public, start_at, end_at, stream_name, key
		) VALUES (
			?, ?, ?, ?, ?, ?
		)`,
		s.DisplayName, s.IsPublic, s.StartAt, s.EndAt, s.StreamName, s.Key,
	)
	if err != nil {
		rerr := tx.Rollback()
		if rerr != nil {
			log.Fatalf("Error rolling back failed transaction: %s", err.Error())
			return rerr
		}
		return err
	}
	cerr := tx.Commit()
	if cerr != nil {
		log.Errorf("Error comitting transaction: %s", err.Error())
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		log.Errorf("Error retrieving ID of inserted row: %s", err.Error())
		return nil // Todo: maybe not return 200 here.
	}

	s.ID = int(id)

	err = json.NewEncoder(w).Encode(&s)
	if err != nil {
		log.Errorf("Error encoding json: %s" + err.Error())
	}

	return nil
}

type config struct {
	DBURL      string
	ListenAddr string
	Verbose    bool
}

func main() {
	log.Info("Starting up")

	// Flags
	var configFile = *flag.String("config", "config.toml", "Path to config file")
	var verbose = *flag.Bool("verbose", false, "Show debug messages")
	flag.Parse()

	// Parse config
	var conf config
	if _, err := toml.DecodeFile(configFile, &conf); err != nil {
		log.Fatalf("Error reading config file: %s", err.Error())
	}

	if verbose || conf.Verbose {
		log.SetLevel(log.DebugLevel)
		log.Debug("Being verbose...")
	}

	// Parse DB URL
	dburl, err := url.Parse(conf.DBURL)
	if err != nil {
		log.Fatalf("Error parsing database URL: %s" + err.Error())
	}

	// Connect to DB...
	var (
		dbDriver, dbInfo string
	)
	switch dburl.Scheme {
	case "sqlite3":
		dbDriver, dbInfo = "sqlite3", dburl.Host+dburl.Path
	case "postgresql":
		dbDriver, dbInfo = "postgres", dburl.String()
	}
	log.Infof("Connecting to db with driver %s and details %s...", dbDriver, dbInfo)
	db, err := sqlx.Connect(dbDriver, dbInfo)
	if err != nil {
		log.Fatalf("Error connecting to DB: %s", err.Error())
	}

	commonHandlers := alice.New(
		logRequestMiddleware,
		handlers.CORS(
			//handlers.AllowedOrigins([]string{"*"}),
			handlers.AllowedMethods([]string{"GET", "POST", "DELETE", "OPTIONS"}),
		),
	)

	router := mux.NewRouter()
	router.Handle("/streams", appHandler{&env{db}, getStreamHandler}).Methods("GET")
	router.Handle("/streams/{id}", appHandler{&env{db}, getStreamHandler}).Methods("GET")
	router.Handle("/streams/{id}", appHandler{&env{db}, deleteStreamHandler}).Methods("DELETE")
	router.Handle("/streams", appHandler{&env{db}, createStreamHandler}).Methods("POST")

	log.Infof("Listening on %s", conf.ListenAddr)
	err = http.ListenAndServe(conf.ListenAddr, commonHandlers.Then(router))
	if err != nil {
		log.Fatalf("Error starting server: %s", err.Error())
	}
}
