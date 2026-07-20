package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    "sync/atomic"
    "time"

    "github.com/google/uuid"
    "github.com/joho/godotenv"
    _ "github.com/lib/pq"
    "github.com/jjusann/chirpy/internal/database"
    "github.com/jjusann/chirpy/internal/auth"
)

type apiConfig struct {
    fileserverHits atomic.Int32
    dbQueries      *database.Queries
    platform       string
}

type User struct {
    ID        uuid.UUID `json:"id"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    Email     string    `json:"email"`
    HashedPassword string    `json:"-"`
}

type Chirp struct {
    ID        uuid.UUID `json:"id"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    Body      string    `json:"body"`
    UserID    uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        cfg.fileserverHits.Add(1)
        next.ServeHTTP(w, r)
    })
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
    // Only allow in dev environment
    if cfg.platform != "dev" {
        w.WriteHeader(http.StatusForbidden)
        w.Write([]byte("Forbidden"))
        return
    }

    // Delete all users from the database
    err := cfg.dbQueries.DeleteAllUsers(r.Context())
    if err != nil {
        log.Printf("Error deleting users: %v", err)
        w.WriteHeader(http.StatusInternalServerError)
        w.Write([]byte("Failed to reset users"))
        return
    }

    // Reset the hit counter (existing behavior)
    cfg.fileserverHits.Store(0)
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("Hits reset"))
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    response := map[string]string{"error": msg}
    data, _ := json.Marshal(response)
    w.Write(data)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    data, err := json.Marshal(payload)
    if err != nil {
        log.Println("Failed to marshal JSON: %v", err)
        w.WriteHeader(http.StatusInternalServerError)
        w.Write([]byte(`{"error": "Internal Server Error"}`))
        return
    }
    w.Write(data)
}

// helper function to replace profanity in chirp body with asterisks
func replaceProfaneWords(text string) string {
    // list of prafane workds to filter out
    profaneWords := map[string]bool{
        "kerfuffle": true,
        "sharbert":  true,
        "fornax":    true,
        // Add more profane words as needed
    }

    words := strings.Split(text, " ")

    for i, word := range words {
        lowerWord := strings.ToLower(word)
        if profaneWords[lowerWord] {
            words[i] = "****" // replace profane word with asterisks
        }
    }
    return strings.Join(words, " ")
}

func main() {

    //Load environment variables from .env file

    err := godotenv.Load()
    if err != nil {
        log.Println("Error loading .env file, proceeding with system environment variables")
    }

    //Get the database URL from environment variables
    dbURL := os.Getenv("DB_URL")
    if dbURL == "" {
        log.Fatal("DB_URL environment variable is not set")
    }

    platform := os.Getenv("PLATFORM")
    if platform == "" {
        platform = "dev" // default to dev if not set
    }

    //Connect to the database
    db, err := sql.Open("postgres", dbURL)
    if err != nil {
        log.Fatalf("Failed to connect to the database: %v", err)
    }
    defer db.Close()

    //Create a new ServeMux
    apiCfg := &apiConfig{
        dbQueries: database.New(db),
        platform:  platform,
    }
    mux := http.NewServeMux()

    //add readiness endpoint
    mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    })

    mux.HandleFunc("GET /admin/metrics", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        html := fmt.Sprintf(`<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, apiCfg.fileserverHits.Load())
        w.Write([]byte(html))
    })

    // add a file server handler for the root path
    fileServer := http.FileServer(http.Dir("."))
    mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", fileServer)))

    //add logo to be accessible optional as it is being already served
    //mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))

    mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)

    mux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request) {
        // Parse request body
        var request struct {
            Body   string `json:"body"`
            UserID string `json:"user_id"`
        }
        if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid request body")
            return
        }

        // Validate: chirp length <= 140 characters
        const maxChirpLength = 140
        if len(request.Body) > maxChirpLength {
            respondWithError(w, http.StatusBadRequest, "Chirp is too long")
            return
        }

        // Validate: user_id is a valid UUID
        userID, err := uuid.Parse(request.UserID)
        if err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid user_id format")
            return
        }

        // Clean profanity
        cleanedBody := replaceProfaneWords(request.Body)

        // Create chirp in database
		dbChirp, err := apiCfg.dbQueries.CreateChirp(r.Context(), database.CreateChirpParams{
			Body:   cleanedBody,
			UserID: userID,
		})        
		
		if err != nil {
            // Check if user exists (foreign key violation)
            if strings.Contains(err.Error(), "foreign key") {
                respondWithError(w, http.StatusNotFound, "User not found")
                return
            }
            respondWithError(w, http.StatusInternalServerError, "Failed to create chirp")
            return
        }

        // Map to main.Chirp struct for JSON response
        chirp := Chirp{
            ID:        dbChirp.ID,
            CreatedAt: dbChirp.CreatedAt,
            UpdatedAt: dbChirp.UpdatedAt,
            Body:      dbChirp.Body,
            UserID:    dbChirp.UserID,
        }

        respondWithJSON(w, http.StatusCreated, chirp)
    })

    mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
        // Parse request body
        var request struct {
            Email    string `json:"email"`
            Password string `json:"password"`
        }
        if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid request body")
            return
        }

        // Validate email
        if request.Email == "" {
            respondWithError(w, http.StatusBadRequest, "Email is required")
            return
        }

        // Validate password (at least 6 characters, good practice)
        if len(request.Password) < 6 {
            respondWithError(w, http.StatusBadRequest, "Password must be at least 6 characters")
            return
        }

        // Hash the password
        hashedPassword, err := auth.HashPassword(request.Password)
        if err != nil {
            respondWithError(w, http.StatusInternalServerError, "Failed to hash password")
            return
        }

        // Create user in database
        dbUser, err := apiCfg.dbQueries.CreateUser(r.Context(), database.CreateUserParams{
            Email:          request.Email,
            HashedPassword: hashedPassword,
        })
        if err != nil {
            // Check for duplicate email error
            if strings.Contains(err.Error(), "duplicate key") {
                respondWithError(w, http.StatusConflict, "Email already exists")
                return
            }
            respondWithError(w, http.StatusInternalServerError, "Failed to create user")
            return
        }

        // Map to main.User struct for JSON response (without password)
        user := User{
            ID:        dbUser.ID,
            CreatedAt: dbUser.CreatedAt,
            UpdatedAt: dbUser.UpdatedAt,
            Email:     dbUser.Email,
        }

        respondWithJSON(w, http.StatusCreated, user)
    })

        mux.HandleFunc("GET /api/chirps", func(w http.ResponseWriter, r *http.Request) {
        // Get all chirps from database
        dbChirps, err := apiCfg.dbQueries.GetChirps(r.Context())
        if err != nil {
            respondWithError(w, http.StatusInternalServerError, "Failed to retrieve chirps")
            return
        }

        // Map database chirps to main.Chirp structs
        chirps := make([]Chirp, 0, len(dbChirps))
        for _, dbChirp := range dbChirps {
            chirps = append(chirps, Chirp{
                ID:        dbChirp.ID,
                CreatedAt: dbChirp.CreatedAt,
                UpdatedAt: dbChirp.UpdatedAt,
                Body:      dbChirp.Body,
                UserID:    dbChirp.UserID,
            })
        }

        respondWithJSON(w, http.StatusOK, chirps)
    })


    mux.HandleFunc("GET /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
        // Get the chirp ID from the path parameter
        chirpIDStr := r.PathValue("chirpID")
        
        // Parse the UUID
        chirpID, err := uuid.Parse(chirpIDStr)
        if err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid chirp ID")
            return
        }

        // Get the chirp from the database
        dbChirp, err := apiCfg.dbQueries.GetChirpByID(r.Context(), chirpID)
        if err != nil {
            // Check if the error is "no rows" (chirp not found)
            if strings.Contains(err.Error(), "no rows") {
                respondWithError(w, http.StatusNotFound, "Chirp not found")
                return
            }
            respondWithError(w, http.StatusInternalServerError, "Failed to retrieve chirp")
            return
        }

        // Map to main.Chirp struct
        chirp := Chirp{
            ID:        dbChirp.ID,
            CreatedAt: dbChirp.CreatedAt,
            UpdatedAt: dbChirp.UpdatedAt,
            Body:      dbChirp.Body,
            UserID:    dbChirp.UserID,
        }

        respondWithJSON(w, http.StatusOK, chirp)
    })

    mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
        // Parse request body
        var request struct {
            Email    string `json:"email"`
            Password string `json:"password"`
        }
        if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid request body")
            return
        }

        // Look up user by email
        dbUser, err := apiCfg.dbQueries.GetUserByEmail(r.Context(), request.Email)
        if err != nil {
            // User not found
            if strings.Contains(err.Error(), "no rows") {
                respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
                return
            }
            respondWithError(w, http.StatusInternalServerError, "Failed to retrieve user")
            return
        }

        // Check password
        valid, err := auth.CheckPasswordHash(request.Password, dbUser.HashedPassword)
        if err != nil || !valid {
            respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
            return
        }

        // Return user (without password)
        user := User{
            ID:        dbUser.ID,
            CreatedAt: dbUser.CreatedAt,
            UpdatedAt: dbUser.UpdatedAt,
            Email:     dbUser.Email,
        }

        respondWithJSON(w, http.StatusOK, user)
    })


    //Create a new Server struct
    server := http.Server{
        Addr:    ":8080",
        Handler: mux,
    }

    //Start the server
    log.Println("Server starting on http://localhost:8080")
    err = server.ListenAndServe()
    if err != nil {
        log.Fatal(err)
    }
}