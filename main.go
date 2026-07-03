package main

import (
    "log"
    "net/http"
	"sync/atomic"
	"fmt"
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

func main() {

    //Create a new ServeMux
	apiCfg := &apiConfig{}
    mux := http.NewServeMux()


	//add readiness endpoint
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request){
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})	

	// add a file server handler for the root path
	fileServer := http.FileServer(http.Dir("."))
    mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", fileServer)))

	//add logo to be accessible optional as it is being already served
	//mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))

	mux.HandleFunc("/metrics", apiCfg.metricsHandler)


	mux.HandleFunc("/reset", apiCfg.resetHandler)

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