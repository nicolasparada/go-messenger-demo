package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/joho/godotenv"
	"github.com/knq/jwt"
	_ "github.com/lib/pq"
	"github.com/matryer/way"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

var origin *url.URL
var db *sql.DB
var githubOAuthConfig oauth2.Config
var cookieSigner *securecookie.SecureCookie
var jwtSigner jwt.Signer
var messageBus *MessageBus

func main() {
	godotenv.Load()

	port := intEnv("PORT", 3000)
	originString := env("ORIGIN", fmt.Sprintf("http://localhost:%d/", port))
	databaseURL := env("DATABASE_URL", "postgresql://root@127.0.0.1:26257/messenger?sslmode=disable")
	githubClientID := os.Getenv("GITHUB_CLIENT_ID")
	githubClientSecret := os.Getenv("GITHUB_CLIENT_SECRET")
	hashKey := env("HASH_KEY", "secret")
	jwtKey := env("JWT_KEY", "secret")

	var err error
	if origin, err = url.Parse(originString); err != nil || !origin.IsAbs() {
		log.Fatal("invalid origin")
		return
	}

	if i, err := strconv.Atoi(origin.Port()); err == nil {
		port = i
	}

	if githubClientID == "" || githubClientSecret == "" {
		log.Fatalf("remember to set both $GITHUB_CLIENT_ID and $GITHUB_CLIENT_SECRET")
		return
	}

	if db, err = sql.Open("postgres", databaseURL); err != nil {
		log.Fatalf("could not open database connection: %v\n", err)
		return
	}
	defer db.Close()
	if err = db.Ping(); err != nil {
		log.Fatalf("could not ping to db: %v\n", err)
		return
	}

	githubRedirectURL := *origin
	githubRedirectURL.Path = "/api/oauth/github/callback"
	githubOAuthConfig = oauth2.Config{
		ClientID:     githubClientID,
		ClientSecret: githubClientSecret,
		Endpoint:     github.Endpoint,
		RedirectURL:  githubRedirectURL.String(),
		Scopes:       []string{"read:user"},
	}

	cookieSigner = securecookie.New([]byte(hashKey), nil).MaxAge(0)

	jwtSigner, err = jwt.HS256.New([]byte(jwtKey))
	if err != nil {
		log.Fatalf("could not create JWT signer: %v\n", err)
		return
	}

	messageBus = &MessageBus{Clients: make(map[MessageClient]struct{})}

	router := way.NewRouter()
	router.HandleFunc("POST", "/api/login", requireJSON(login))
	router.HandleFunc("GET", "/api/oauth/github", githubOAuthStart)
	router.HandleFunc("GET", "/api/oauth/github/callback", githubOAuthCallback)
	router.HandleFunc("GET", "/api/auth_user", guard(getAuthUser))
	router.HandleFunc("POST", "/api/refresh_token", guard(refreshToken))
	router.HandleFunc("GET", "/api/usernames", guard(searchUsernames))
	router.HandleFunc("POST", "/api/conversations", requireJSON(guard(createConversation)))
	router.HandleFunc("GET", "/api/conversations", guard(getConversations))
	router.HandleFunc("GET", "/api/conversations/:conversation_id", guard(getConversation))
	router.HandleFunc("POST", "/api/conversations/:conversation_id/messages", requireJSON(guard(createMessage)))
	router.HandleFunc("GET", "/api/conversations/:conversation_id/messages", guard(getMessages))
	router.HandleFunc("GET", "/api/messages", guard(subscribeToMessages))
	router.HandleFunc("POST", "/api/conversations/:conversation_id/read_messages", guard(readMessages))
	router.HandleFunc("*", "/api/...", http.NotFound)
	router.Handle("GET", "/...", http.FileServer(SPAFileSystem{http.Dir("static")}))

	s := http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           router,
		ReadHeaderTimeout: time.Second * 5,
		IdleTimeout:       time.Second * 30,
	}

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt)
		<-quit
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			log.Fatalf("could not shutdown server: %v\n", err)
		}
	}()

	log.Printf("accepting connections on port %d\n", port)
	log.Printf("starting server at %s\n", origin.String())
	if err := s.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("could not start server: %v\n", err)
	}
}

func env(key, fallbackValue string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallbackValue
	}
	return v
}

func intEnv(key string, fallbackValue int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallbackValue
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallbackValue
	}
	return i
}
