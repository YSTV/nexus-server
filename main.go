package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"

	"github.com/BurntSushi/toml"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	_ "github.com/joho/godotenv/autoload"
	"github.com/justinas/alice"
	_ "github.com/lib/pq"
)

func imAnIdiot(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request: %s", r.URL.String())
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
		switch e := err.(type) {
		case appError:
			http.Error(w, e.Error(), e.Status())
		default:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	}
}

func listStreamsHandler(e *env, w http.ResponseWriter, r *http.Request) error {
	var streams []stream
	err := e.db.Select(&streams, `
	SELECT id, name, is_public, start_at, end_at`)
	if err != nil {
		return err
	}

	json.NewEncoder(w).Encode(streams)
	return nil
}

type config struct {
	DBURL      string
	ListenAddr string
}

func main() {
	log.Info("Starting up")

	// Flags
	var configFile = *flag.String("config", "config.toml", "Path to config file")
	flag.Parse()

	// Parse config
	var conf config
	if _, err := toml.DecodeFile(configFile, &conf); err != nil {
		log.Fatalf("Error reading config file: %s", err.Error())
	}

	// Parse DB URL
	dburl, err := url.Parse(conf.DBURL)
	if err != nil {
		log.Fatalf("Error parsing database URL: %s" + err.Error())
	}

	// Connect to DB...
	db, err := sqlx.Connect("postgres", dburl.String())
	if err != nil {
		log.Fatalf("Error connecting to DB: %s", err.Error())
	}

	commonHandlers := alice.New(imAnIdiot)

	router := mux.NewRouter()
	router.Handle("/streams", appHandler{&env{db}, listStreamsHandler}).Methods("GET")

	log.Infof("Listening on %s", conf.ListenAddr)
	err = http.ListenAndServe(conf.ListenAddr, commonHandlers.Then(router))
	if err != nil {
		log.Fatalf("Error starting server: %s", err.Error())
	}
}
