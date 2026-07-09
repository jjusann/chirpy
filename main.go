package main

import (
    "log"
    "net/http"
	"sync/atomic"
	"fmt"
	"encoding/json"
	"strings"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request){
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
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
		"sharbert": true,
		"fornax": true,
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

    //Create a new ServeMux
	apiCfg := &apiConfig{}
    mux := http.NewServeMux()


	//add readiness endpoint
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request){
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

	mux.HandleFunc("POST /api/validate_chirp", func(w http.ResponseWriter, r *http.Request) {

		// read the request body
		var request struct {
			Body string `json:"body"`
		}

		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&request); err != nil {
			respondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// check if the chirp is too long

		const maxChirpLength = 140
		if len(request.Body) > maxChirpLength {
			respondWithError(w, http.StatusBadRequest, "Chirp is too long")
			return
		}

		// check for profanity
		cleanedBody := replaceProfaneWords(request.Body)

		//Respond with the cleaned chirp body
		respondWithJSON(w, http.StatusOK, map[string]string{"cleaned_body": cleanedBody})
	})

    //Create a new Server struct
    server := http.Server{
        Addr:    ":8080",
        Handler: mux,
    }



    //Start the server
    log.Println("Server starting on http://localhost:8080")
    err := server.ListenAndServe()
    if err != nil {
        log.Fatal(err)
    }
}