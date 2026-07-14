package main

import (
	_ "github.com/lib/pq"
	"github.com/joho/godotenv"
	"database/sql"
	"os"
	"log"
	"net/http"
	"sync/atomic"
	"fmt"
	"encoding/json"
	"strings"
	"github.com/yengso/boot-go-server/internal/database"
	"github.com/yengso/boot-go-server/internal/auth"
	"time"
	"github.com/google/uuid"
)


type apiConfig struct {
	fileserverHits 	atomic.Int32
	db				*database.Queries
	platform		string
}

type User struct {
	ID				uuid.UUID 	`json:"id"`
	CreatedAt		time.Time 	`json:"created_at"`
	UpdatedAt		time.Time 	`json:"updated_at"`
	Email			string	  	`json:"email"`
	HashedPassword	string		`json:"-"`
}

type Chirp struct {
	ID 			uuid.UUID 	`json:"id"`
	CreatedAt	time.Time 	`json:"created_at"`
	UpdatedAt	time.Time 	`json:"updated_at"`
	Body		string		`json:"body"`
	UserID		uuid.UUID	`json:"user_id"`
}


func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	hits := fmt.Sprintf(`
	<html>
  		<body>
    		<h1>Welcome, Chirpy Admin</h1>
    		<p>Chirpy has been visited %d times!</p>
  		</body>
	</html>`, cfg.fileserverHits.Load())
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(hits))
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("..."))
		return
	}

	err := cfg.db.DeleteAllUsers(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Unable to delete all users"))
		return
	}

	cfg.fileserverHits.Store(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type returnVals struct {
		Error string `json:"error"`
	}

	respBody := returnVals{
		Error: msg,
	}
	respondWithJSON(w, code, respBody)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}


func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)
	apiCfg := apiConfig{
		db: dbQueries,
		platform: platform,
	}

	mux := http.NewServeMux()
	
	s := &http.Server{
		Addr:			":8080",
		Handler:		mux,
		MaxHeaderBytes: 1 << 20,
	}
	
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))

	})

	mux.HandleFunc("GET /api/chirps", func(w http.ResponseWriter, r *http.Request){
		allDatabaseChirps, err := apiCfg.db.GetAllChirps(r.Context())
		if err != nil {
			respondWithError(w, 400, "Could not get all Chirps from database")
			return
		}

		allChirps := []Chirp{} 

		for _, chirp := range allDatabaseChirps {
			allChirps = append(allChirps, Chirp{
				ID: 		chirp.ID,
				CreatedAt:	chirp.CreatedAt,
				UpdatedAt:	chirp.UpdatedAt,
				Body:		chirp.Body,
				UserID:		chirp.UserID,
			})
		}
		respondWithJSON(w, http.StatusOK, allChirps)
	})

	mux.HandleFunc("GET /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request){
		chirpID, err := uuid.Parse(r.PathValue("chirpID"))
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Could not Parse chirpID string to uuid type")
			return
		}

		singleChirp, err := apiCfg.db.GetSingleChirp(r.Context(), chirpID)
		if err != nil {
			respondWithError(w, http.StatusNotFound, "Could not find the Chirp ID")
			return
		}

		respChirp := Chirp{
			ID: 		singleChirp.ID,
			CreatedAt:	singleChirp.CreatedAt,
			UpdatedAt:	singleChirp.UpdatedAt,
			Body:		singleChirp.Body,
			UserID:		singleChirp.UserID,
		}
		respondWithJSON(w, http.StatusOK, respChirp)
	})

	mux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request){
		type parameters struct {
			Body 	string 		`json:"body"`
			UserID 	uuid.UUID 	`json:"user_id"` 
		}

		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			respondWithError(w, 400, "Something went wrong")
			return
		}
		if len(params.Body) > 140 {
			respondWithError(w, 400, "Chirp is too long")
			return
		}

		badWords := map[string]struct{}{
			"kerfuffle": {},
			"sharbert": {},
			"fornax": {},
		}
		
		sentence := strings.Split(params.Body, " ")
		for i, word := range sentence {
			if _, ok := badWords[strings.ToLower(word)]; ok {
				sentence[i] = "****"
			}
		}
		cleanedSentence := strings.Join(sentence, " ")

		chirpParams := database.CreateChirpParams{
			Body:	cleanedSentence,
			UserID:	params.UserID,
		}

		dbChirp, err := apiCfg.db.CreateChirp(r.Context(), chirpParams)
		if err != nil {
			respondWithError(w, 400, "Error adding Chirp post to database")
			return
		}

		newChirp := Chirp{
			ID:			dbChirp.ID,
			CreatedAt:	dbChirp.CreatedAt,
			UpdatedAt:	dbChirp.UpdatedAt,
			Body:		dbChirp.Body,
			UserID:		dbChirp.UserID,
		}

		respondWithJSON(w, http.StatusCreated, newChirp)
	})

	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request){
		type parameters struct {
			Password 	string `json:"password"`
			Email 		string `json:"email"`
		}

		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			respondWithError(w, 400, "Something went wrong")
			return
		}

		hPassword, err := auth.HashPassword(params.Password)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Hashing password failed")
			return
		}
		dbUser, err := apiCfg.db.CreateUser(r.Context(), database.CreateUserParams{
			Email: 			params.Email,
			HashedPassword:	hPassword,
		})
		if err != nil {
			respondWithError(w, 500, "Error creating database user")
			return
		}

		newUser := User{
			ID: 			dbUser.ID,
			CreatedAt: 		dbUser.CreatedAt,
			UpdatedAt:		dbUser.UpdatedAt,
			Email:			dbUser.Email,
		}
		respondWithJSON(w, http.StatusCreated, newUser)
	})

	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request){
		type parameters struct {
			Password 	string `json:"password"`
			Email 		string `json:"email"`
		}
		
		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			respondWithError(w, 400, "Something went wrong")
			return
		}

		loginErrMsg := "Incorrect email or password"

		dbUser, err := apiCfg.db.SearchUserByEmail(r.Context(), params.Email)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, loginErrMsg)
			return
		}

		ok, err := auth.CheckPasswordHash(params.Password, dbUser.HashedPassword)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, loginErrMsg)
			return
		}

		loginUser := User{
			ID: 		dbUser.ID,
			CreatedAt:	dbUser.CreatedAt,
			UpdatedAt:	dbUser.UpdatedAt,
			Email:		dbUser.Email,
		}

		if !ok {
			respondWithError(w, http.StatusUnauthorized, loginErrMsg)
			return
		}
		respondWithJSON(w, http.StatusOK, loginUser)
		return
	})

	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)

	log.Fatal(s.ListenAndServe())
}