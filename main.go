package main

import (
    "log"
    "net/http"
)

func main() {
    //Create a new ServeMux
    mux := http.NewServeMux()

	// add a file server handler for the root path
	fileServer := http.FileServer(http.Dir("."))
	mux.Handle("/", fileServer)
	//add logo to be accessible 
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))

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