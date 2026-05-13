package main

import (
	"log"
	"net/http"
	"time"
)



func main() {
	mux := http.NewServeMux()

	s := &http.Server{
		Addr:			":8000",
		Handler:		mux,
		ReadTimeout:	10 * time.Second,
		WriteTimeout:	10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	
	mux.Handle("/", http.FileServer(http.Dir(".")))
	mux.Handle("/assets", http.FileServer(http.Dir(".")))



	log.Fatal(s.ListenAndServe())
}