package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "runtime/debug"
    "strings"
    "sync/atomic"
    "time"

    "github.com/google/uuid"
    "github.com/joho/godotenv"
    _ "github.com/lib/pq"
    "github.com/jjusann/chirpy/internal/auth"
    "github.com/jjusann/chirpy/internal/database"
)

type apiConfig struct {
    fileserverHits atomic.Int32
    dbQueries      *database.Queries
    platform       string
    jwtSecret      string
    polkaKey       string
    rawDB          *sql.DB
}

type User struct {
    ID             uuid.UUID `json:"id"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
    Email          string    `json:"email"`
    IsChirpyRed    bool      `json:"is_chirpy_red"`
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
    if cfg.platform != "dev" {
        w.WriteHeader(http.StatusForbidden)
        w.Write([]byte("Forbidden"))
        return
    }

    err := cfg.dbQueries.DeleteAllUsers(r.Context())
    if err != nil {
        log.Printf("Error deleting users: %v", err)
        w.WriteHeader(http.StatusInternalServerError)
        w.Write([]byte("Failed to reset users"))
        return
    }

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
        log.Printf("Failed to marshal JSON: %v", err)
        w.WriteHeader(http.StatusInternalServerError)
        w.Write([]byte(`{"error": "Internal Server Error"}`))
        return
    }
    w.Write(data)
}

func replaceProfaneWords(text string) string {
    profaneWords := map[string]bool{
        "kerfuffle": true,
        "sharbert":  true,
        "fornax":    true,
    }

    words := strings.Split(text, " ")
    for i, word := range words {
        lowerWord := strings.ToLower(word)
        if profaneWords[lowerWord] {
            words[i] = "****"
        }
    }
    return strings.Join(words, " ")
}

func main() {
    err := godotenv.Load()
    if err != nil {
        log.Println("Error loading .env file, proceeding with system environment variables")
    }

    dbURL := os.Getenv("DB_URL")
    if dbURL == "" {
        log.Fatal("DB_URL environment variable is not set")
    }

    platform := os.Getenv("PLATFORM")
    if platform == "" {
        platform = "dev"
    }

    jwtSecret := os.Getenv("JWT_SECRET")
    if jwtSecret == "" {
        log.Fatal("JWT_SECRET environment variable is not set")
    }

    db, err := sql.Open("postgres", dbURL)
    if err != nil {
        log.Fatalf("Failed to connect to the database: %v", err)
    }
    defer db.Close()

    if err := db.Ping(); err != nil {
        log.Fatalf("Cannot ping database: %v", err)
    }
    log.Println("✅ Database connection verified")

    polkaKey := os.Getenv("POLKA_KEY")
    if polkaKey == "" {
        log.Fatal("POLKA_KEY environment variable is not set")
    }

    apiCfg := &apiConfig{
        dbQueries: database.New(db),
        platform:  platform,
        jwtSecret: jwtSecret,
        polkaKey:  polkaKey,
        rawDB:     db,
    }

    mux := http.NewServeMux()

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

    fileServer := http.FileServer(http.Dir("."))
    mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", fileServer)))

    mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)

    // ---------- CHIRPS ----------
    mux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request) {
        token, err := auth.GetBearerToken(r.Header)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, err.Error())
            return
        }

        userID, err := auth.ValidateJWT(token, apiCfg.jwtSecret)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, "Invalid or expired token")
            return
        }

        var request struct {
            Body string `json:"body"`
        }
        if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid request body")
            return
        }

        const maxChirpLength = 140
        if len(request.Body) > maxChirpLength {
            respondWithError(w, http.StatusBadRequest, "Chirp is too long")
            return
        }

        cleanedBody := replaceProfaneWords(request.Body)

        dbChirp, err := apiCfg.dbQueries.CreateChirp(r.Context(), database.CreateChirpParams{
            Body:   cleanedBody,
            UserID: userID,
        })
        if err != nil {
            if strings.Contains(err.Error(), "foreign key") {
                respondWithError(w, http.StatusNotFound, "User not found")
                return
            }
            respondWithError(w, http.StatusInternalServerError, "Failed to create chirp")
            return
        }

        chirp := Chirp{
            ID:        dbChirp.ID,
            CreatedAt: dbChirp.CreatedAt,
            UpdatedAt: dbChirp.UpdatedAt,
            Body:      dbChirp.Body,
            UserID:    dbChirp.UserID,
        }

        respondWithJSON(w, http.StatusCreated, chirp)
    })

    mux.HandleFunc("GET /api/chirps", func(w http.ResponseWriter, r *http.Request) {
        dbChirps, err := apiCfg.dbQueries.GetChirps(r.Context())
        if err != nil {
            respondWithError(w, http.StatusInternalServerError, "Failed to retrieve chirps")
            return
        }

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
        chirpIDStr := r.PathValue("chirpID")
        chirpID, err := uuid.Parse(chirpIDStr)
        if err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid chirp ID")
            return
        }

        dbChirp, err := apiCfg.dbQueries.GetChirpByID(r.Context(), chirpID)
        if err != nil {
            if strings.Contains(err.Error(), "no rows") {
                respondWithError(w, http.StatusNotFound, "Chirp not found")
                return
            }
            respondWithError(w, http.StatusInternalServerError, "Failed to retrieve chirp")
            return
        }

        chirp := Chirp{
            ID:        dbChirp.ID,
            CreatedAt: dbChirp.CreatedAt,
            UpdatedAt: dbChirp.UpdatedAt,
            Body:      dbChirp.Body,
            UserID:    dbChirp.UserID,
        }

        respondWithJSON(w, http.StatusOK, chirp)
    })

    mux.HandleFunc("DELETE /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
        token, err := auth.GetBearerToken(r.Header)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, err.Error())
            return
        }

        userID, err := auth.ValidateJWT(token, apiCfg.jwtSecret)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, "Invalid or expired token")
            return
        }

        chirpIDStr := r.PathValue("chirpID")
        chirpID, err := uuid.Parse(chirpIDStr)
        if err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid chirp ID")
            return
        }

        dbChirp, err := apiCfg.dbQueries.GetChirpByID(r.Context(), chirpID)
        if err != nil {
            if strings.Contains(err.Error(), "no rows") {
                respondWithError(w, http.StatusNotFound, "Chirp not found")
                return
            }
            respondWithError(w, http.StatusInternalServerError, "Failed to retrieve chirp")
            return
        }

        if dbChirp.UserID != userID {
            respondWithError(w, http.StatusForbidden, "You are not the author of this chirp")
            return
        }

        err = apiCfg.dbQueries.DeleteChirpByID(r.Context(), chirpID)
        if err != nil {
            respondWithError(w, http.StatusInternalServerError, "Failed to delete chirp")
            return
        }

        w.WriteHeader(http.StatusNoContent)
    })

    // ---------- USERS (raw SQL) ----------
    mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
        var request struct {
            Email    string `json:"email"`
            Password string `json:"password"`
        }
        if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid request body")
            return
        }

        if request.Email == "" {
            respondWithError(w, http.StatusBadRequest, "Email is required")
            return
        }
        if len(request.Password) < 4 {
            respondWithError(w, http.StatusBadRequest, "Password must be at least 4 characters")
            return
        }

        hashedPassword, err := auth.HashPassword(request.Password)
        if err != nil {
            respondWithError(w, http.StatusInternalServerError, "Failed to hash password")
            return
        }

        var dbUser struct {
            ID          uuid.UUID
            CreatedAt   time.Time
            UpdatedAt   time.Time
            Email       string
            IsChirpyRed bool
        }
        err = apiCfg.rawDB.QueryRowContext(r.Context(), `
            INSERT INTO users (id, created_at, updated_at, email, hashed_password)
            VALUES (gen_random_uuid(), NOW(), NOW(), $1, $2)
            RETURNING id, created_at, updated_at, email, is_chirpy_red
        `, request.Email, hashedPassword).Scan(
            &dbUser.ID,
            &dbUser.CreatedAt,
            &dbUser.UpdatedAt,
            &dbUser.Email,
            &dbUser.IsChirpyRed,
        )
        if err != nil {
            if strings.Contains(err.Error(), "duplicate key") {
                respondWithError(w, http.StatusConflict, "Email already exists")
                return
            }
            log.Printf("Create user error: %v", err)
            respondWithError(w, http.StatusInternalServerError, "Failed to create user")
            return
        }

        user := User{
            ID:          dbUser.ID,
            CreatedAt:   dbUser.CreatedAt,
            UpdatedAt:   dbUser.UpdatedAt,
            Email:       dbUser.Email,
            IsChirpyRed: dbUser.IsChirpyRed,
        }

        respondWithJSON(w, http.StatusCreated, user)
    })

    mux.HandleFunc("PUT /api/users", func(w http.ResponseWriter, r *http.Request) {
        token, err := auth.GetBearerToken(r.Header)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, err.Error())
            return
        }

        userID, err := auth.ValidateJWT(token, apiCfg.jwtSecret)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, "Invalid or expired token")
            return
        }

        var request struct {
            Email    string `json:"email"`
            Password string `json:"password"`
        }
        if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid request body")
            return
        }

        if request.Email == "" {
            respondWithError(w, http.StatusBadRequest, "Email is required")
            return
        }
        if len(request.Password) < 4 {
            respondWithError(w, http.StatusBadRequest, "Password must be at least 4 characters")
            return
        }

        hashedPassword, err := auth.HashPassword(request.Password)
        if err != nil {
            respondWithError(w, http.StatusInternalServerError, "Failed to hash password")
            return
        }

        var dbUser struct {
            ID          uuid.UUID
            CreatedAt   time.Time
            UpdatedAt   time.Time
            Email       string
            IsChirpyRed bool
        }
        err = apiCfg.rawDB.QueryRowContext(r.Context(), `
            UPDATE users
            SET email = $1, hashed_password = $2, updated_at = NOW()
            WHERE id = $3
            RETURNING id, created_at, updated_at, email, is_chirpy_red
        `, request.Email, hashedPassword, userID).Scan(
            &dbUser.ID,
            &dbUser.CreatedAt,
            &dbUser.UpdatedAt,
            &dbUser.Email,
            &dbUser.IsChirpyRed,
        )
        if err != nil {
            if strings.Contains(err.Error(), "duplicate key") {
                respondWithError(w, http.StatusConflict, "Email already exists")
                return
            }
            log.Printf("Update user error: %v", err)
            respondWithError(w, http.StatusInternalServerError, "Failed to update user")
            return
        }

        user := User{
            ID:          dbUser.ID,
            CreatedAt:   dbUser.CreatedAt,
            UpdatedAt:   dbUser.UpdatedAt,
            Email:       dbUser.Email,
            IsChirpyRed: dbUser.IsChirpyRed,
        }

        respondWithJSON(w, http.StatusOK, user)
    })

    // ---------- LOGIN (raw SQL) ----------
    mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if r := recover(); r != nil {
                log.Printf("🔥 PANIC in login: %v", r)
                debug.PrintStack()
                respondWithError(w, http.StatusInternalServerError, "Internal server error")
            }
        }()

        var request struct {
            Email    string `json:"email"`
            Password string `json:"password"`
        }
        if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid request body")
            return
        }

        var dbUser struct {
            ID             uuid.UUID
            CreatedAt      time.Time
            UpdatedAt      time.Time
            Email          string
            IsChirpyRed    bool
            HashedPassword string
        }
        err := apiCfg.rawDB.QueryRowContext(r.Context(), `
            SELECT id, created_at, updated_at, email, is_chirpy_red, hashed_password
            FROM users
            WHERE email = $1
        `, request.Email).Scan(
            &dbUser.ID,
            &dbUser.CreatedAt,
            &dbUser.UpdatedAt,
            &dbUser.Email,
            &dbUser.IsChirpyRed,
            &dbUser.HashedPassword,
        )
        if err != nil {
            if err == sql.ErrNoRows {
                respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
                return
            }
            log.Printf("Login query error: %v", err)
            respondWithError(w, http.StatusInternalServerError, "Failed to retrieve user")
            return
        }

        valid, err := auth.CheckPasswordHash(request.Password, dbUser.HashedPassword)
        if err != nil || !valid {
            respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
            return
        }

        accessToken, err := auth.MakeJWT(dbUser.ID, apiCfg.jwtSecret, time.Hour)
        if err != nil {
            respondWithError(w, http.StatusInternalServerError, "Failed to generate access token")
            return
        }

        refreshToken, err := auth.MakeRefreshToken()
        if err != nil {
            respondWithError(w, http.StatusInternalServerError, "Failed to generate refresh token")
            return
        }

        expiresAt := time.Now().UTC().Add(60 * 24 * time.Hour)
        _, err = apiCfg.rawDB.ExecContext(r.Context(), `
            INSERT INTO refresh_tokens (token, created_at, updated_at, user_id, expires_at, revoked_at)
            VALUES ($1, NOW(), NOW(), $2, $3, NULL)
        `, refreshToken, dbUser.ID, expiresAt)
        if err != nil {
            log.Printf("Insert refresh token error: %v", err)
            respondWithError(w, http.StatusInternalServerError, "Failed to store refresh token")
            return
        }

        user := User{
            ID:          dbUser.ID,
            CreatedAt:   dbUser.CreatedAt,
            UpdatedAt:   dbUser.UpdatedAt,
            Email:       dbUser.Email,
            IsChirpyRed: dbUser.IsChirpyRed,
        }

        response := struct {
            User
            Token        string `json:"token"`
            RefreshToken string `json:"refresh_token"`
        }{
            User:         user,
            Token:        accessToken,
            RefreshToken: refreshToken,
        }

        respondWithJSON(w, http.StatusOK, response)
    })

    // ---------- REFRESH & REVOKE (use sqlc, but may also need raw SQL) ----------
    mux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
        refreshToken, err := auth.GetBearerToken(r.Header)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, "Missing or invalid authorization header")
            return
        }

        tokenData, err := apiCfg.dbQueries.GetUserFromRefreshToken(r.Context(), refreshToken)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, "Invalid, expired, or revoked refresh token")
            return
        }

        newAccessToken, err := auth.MakeJWT(tokenData.ID, apiCfg.jwtSecret, time.Hour)
        if err != nil {
            respondWithError(w, http.StatusInternalServerError, "Failed to generate access token")
            return
        }

        respondWithJSON(w, http.StatusOK, map[string]string{"token": newAccessToken})
    })

    mux.HandleFunc("POST /api/revoke", func(w http.ResponseWriter, r *http.Request) {
        refreshToken, err := auth.GetBearerToken(r.Header)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, "Missing or invalid authorization header")
            return
        }

        _, err = apiCfg.dbQueries.GetUserFromRefreshToken(r.Context(), refreshToken)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, "Invalid, expired, or revoked refresh token")
            return
        }

        err = apiCfg.dbQueries.RevokeRefreshToken(r.Context(), refreshToken)
        if err != nil {
            respondWithError(w, http.StatusInternalServerError, "Failed to revoke refresh token")
            return
        }

        w.WriteHeader(http.StatusNoContent)
    })

    // ---------- POLKA WEBHOOK (raw SQL) ----------
    mux.HandleFunc("POST /api/polka/webhooks", func(w http.ResponseWriter, r *http.Request) {
        // 1. Validate API key
        apiKey, err := auth.GetAPIKey(r.Header)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, err.Error())
            return
        }

        if apiKey != apiCfg.polkaKey {
            respondWithError(w, http.StatusUnauthorized, "Invalid API key")
            return
        }

        // 2. Parse request body
        var req struct {
            Event string `json:"event"`
            Data   struct {
                UserID string `json:"user_id"`
            } `json:"data"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid request body")
            return
        }

        // 3. Ignore any event other than user.upgraded
        if req.Event != "user.upgraded" {
            w.WriteHeader(http.StatusNoContent)
            return
        }

        // 4. Validate user ID format
        userID, err := uuid.Parse(req.Data.UserID)
        if err != nil {
            respondWithError(w, http.StatusBadRequest, "Invalid user_id format")
            return
        }

        // 5. Upgrade user to Chirpy Red
        result, err := apiCfg.rawDB.ExecContext(r.Context(), `
            UPDATE users
            SET is_chirpy_red = TRUE, updated_at = NOW()
            WHERE id = $1
        `, userID)
        if err != nil {
            log.Printf("Upgrade user error: %v", err)
            respondWithError(w, http.StatusInternalServerError, "Failed to upgrade user")
            return
        }

        rowsAffected, _ := result.RowsAffected()
        if rowsAffected == 0 {
            respondWithError(w, http.StatusNotFound, "User not found")
            return
        }

        w.WriteHeader(http.StatusNoContent)
    })

    // ---------- SERVER ----------
    server := http.Server{
        Addr:    ":8080",
        Handler: mux,
    }

    log.Println("🚀 Server starting on http://localhost:8080")
    err = server.ListenAndServe()
    if err != nil {
        log.Fatal(err)
    }
}