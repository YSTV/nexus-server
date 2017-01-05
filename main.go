package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"

	"github.com/BurntSushi/toml"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/justinas/alice"

	"github.com/mattes/migrate/file"
	"github.com/mattes/migrate/migrate"
	"github.com/mattes/migrate/migrate/direction"
	"github.com/mattes/migrate/pipe"

	_ "github.com/mattes/migrate/driver/ql"
	_ "github.com/cznic/ql"

	log "github.com/sirupsen/logrus"

)

const DBFILENAME = "data.db"

var VERSION = "" // Should be automatically inserted by linker during build

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
	db                              *sqlx.DB
	updatesWSHub, streamStatusWSHub *Hub
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

func writeError(w http.ResponseWriter, _ *http.Request, err error) {
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
			id() as id, display_name, is_public, start_at, end_at, stream_name, key
		FROM
			streams
	`

	var err error
	vars := mux.Vars(r)

	if id, ok := vars["id"]; ok { // Specific id
		var s stream
		err := e.db.Get(&s, streamSQL+`WHERE id()=$1`, id)
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
		DELETE FROM streams WHERE id()=$1
	`

	id, ok := mux.Vars(r)["id"]
	if !ok { // No id in URL. Shouldn't happen as mux does matching
		return statusError{
			400,
			errors.New("No id in URL"),
		}
	}
	intID, err := strconv.Atoi(id)
	if err != nil {
		return statusError{
			400,
			errors.New("Non-numeric ID in URL"),
		}
	}

	tx, err := e.db.Begin()
	if err != nil {
		return err
	}
	result, err := tx.Exec(deleteSQL, int64(intID))
	if err != nil {
		if err := tx.Rollback(); err != nil {
			return err
		}
		return err
	}

	if err := tx.Commit(); err != nil {
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
			$1, $2, $3, $4, $5, $6
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

// Handle requests originating from nginx-rtmp's on_publish feature. Validate a stream's name and key
// against the database.
func rpcHandleStreamHandler(e *env, w http.ResponseWriter, r *http.Request) error {
	var s stream
	if r.FormValue("name") == "" {
		return statusError{
			400,
			errors.New("No stream name"),
		}
	}
	err := e.db.Get(&s, "SELECT * FROM streams WHERE stream_name = $1", r.FormValue("name"))
	if err == sql.ErrNoRows {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	} else if err != nil {
		return err
	}
	if s.Key != r.FormValue("key") { // The key passed in the stream URL
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	return nil
}

func updatesHandler(e *env, w http.ResponseWriter, r *http.Request) error {
	if err := e.updatesWSHub.handleRequest(w, r); err != nil {
		return err
	}
	return nil
}

func streamStatusHandler(e *env, w http.ResponseWriter, r *http.Request) error {
	if err := e.streamStatusWSHub.handleRequest(w, r); err != nil {
		return err
	}
	return nil
}

func runMigrations(dbURL, migrationsPath string) {
	log.Info("Applying migrations")
	pipe := pipe.New()
	go migrate.Up(pipe, "ql+" + dbURL, migrationsPath)
	ok := true
	OuterLoop:
	for {
		select {
		case item, more := <-pipe:
			if !more {
				break OuterLoop
			}
			switch item.(type) {
			case error:
				log.Errorf("Error migrating: %s", item.(error).Error())
				ok = false
			case file.File:
				f:= item.(file.File)
				var dir string
				if f.Direction == direction.Up {
					dir = "up"
				} else if f.Direction == direction.Down {
					dir = "down"
				}
				log.Infof("Applying %s migration: %s", dir, f.Name)
			}
		}
	}
	if !ok {
		log.Fatal("Errors while migrating database. See above for details")
	}

}

type config struct {
	Log struct {
		Verbose bool // Make more logging noise
	}
	API struct {
		Listen string // Listen address of HTTP API server
	}
	Data struct {
		MigrationsDir string
		Dir string // Path to directory to store db
	}
}

func main() {
	// Flags
	var configFile = *flag.String("config", "config.toml", "Path to config file")
	var verbose = flag.Bool("verbose", false, "Show debug messages")
	var version = flag.Bool("version", false, "Show version")
	flag.Parse()

	if *version == true {
		fmt.Printf("Nexus Server %s\n", VERSION)
		os.Exit(0)
	}

	// Parse config
	var conf config
	if _, err := toml.DecodeFile(configFile, &conf); err != nil {
		log.Fatalf("Error reading config file: %s", err.Error())
	}

	if (*verbose == true) || conf.Log.Verbose {
		log.SetLevel(log.DebugLevel)
		log.Debug("Being verbose...")
	}
	if !path.IsAbs(conf.Data.Dir) {
		log.Warnf("Using relative path to data directory: %s", conf.Data.Dir)
	}

	dbURL := "file://"+path.Join(conf.Data.Dir, DBFILENAME)

	// Apply database migrations
	runMigrations(dbURL, conf.Data.MigrationsDir)

	db, err := sqlx.Connect("ql", dbURL)
	if err != nil {
		log.Fatalf("Error connecting to DB: %s", err.Error())
	}

	e := &env{
		db:                db,
		updatesWSHub:      newHub(),
		streamStatusWSHub: newHub(),
	}

	// For now, broadcast any stream status updates to all clients
	e.streamStatusWSHub.setIncomingHandler(func(m *Message) {
		log.Infof("Message from %s: %s", m.remoteAddr, string(m.data))
		e.updatesWSHub.broadcast <- m.data
	})

	go e.updatesWSHub.run()
	go e.streamStatusWSHub.run()

	commonHandlers := alice.New(
		logRequestMiddleware,
		handlers.CORS(
			//handlers.AllowedOrigins([]string{"*"}),
			handlers.AllowedMethods([]string{"GET", "POST", "DELETE", "OPTIONS"}),
		),
	)

	router := mux.NewRouter()
	router.Handle("/v1/ws/updates", appHandler{e, updatesHandler})
	router.Handle("/v1/ws/streamstatus", appHandler{e, streamStatusHandler})

	apiRouter := router.PathPrefix("/v1/api/").Subrouter()
	apiRouter.Handle("/streams", appHandler{e, getStreamHandler}).Methods("GET")
	apiRouter.Handle("/streams/{id}", appHandler{e, getStreamHandler}).Methods("GET")
	apiRouter.Handle("/streams/{id}", appHandler{e, deleteStreamHandler}).Methods("DELETE")
	apiRouter.Handle("/streams", appHandler{e, createStreamHandler}).Methods("POST")

	rpcRouter := router.PathPrefix("/v1/rpc/").Subrouter()
	rpcRouter.Handle("/handle_stream", appHandler{e, rpcHandleStreamHandler})

	log.Infof("Listening on %s", conf.API.Listen)
	err = http.ListenAndServe(conf.API.Listen, commonHandlers.Then(router))
	if err != nil {
		log.Fatalf("Error starting server: %s", err.Error())
	}
}
