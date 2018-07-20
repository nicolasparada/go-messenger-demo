package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

var keyAuthUserID = ContextKey{"auth_user_id"}

// ContextKey used for middleware.
type ContextKey struct {
	Name string
}

// SPAFileSystem file system with single-page applications support.
type SPAFileSystem struct {
	fs http.FileSystem
}

// Errors response.
type Errors struct {
	Errors map[string]string `json:"errors"`
}

// Open wraps http.Dir .Open() method to enable single-page applications.
func (spa SPAFileSystem) Open(name string) (http.File, error) {
	f, err := spa.fs.Open(name)
	if err != nil {
		return spa.fs.Open("index.html")
	}
	return f, nil
}

func requireJSON(handler http.HandlerFunc) http.HandlerFunc {
	required := func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			http.Error(w, "Content type of application/json required", http.StatusUnsupportedMediaType)
			return
		}
		handler(w, r)
	}
	return required
}

func respond(w http.ResponseWriter, v interface{}, statusCode int) {
	b, err := json.Marshal(v)
	if err != nil {
		respondError(w, fmt.Errorf("could not marshal response: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	w.Write(b)
}

func respondError(w http.ResponseWriter, err error) {
	log.Println(err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
