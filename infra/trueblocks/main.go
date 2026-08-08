package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
)


type StatusResponse struct {
	Data []StatusData `json:"data"`
}

type StatusData struct {
	ClientBlock  int64 `json:"clientBlock"`
	ScrapeBlock  int64 `json:"scrapeBlock"`
	IndexedBlock int64 `json:"indexedBlock"`
}

type ExportResponse struct {
	Data []TxData `json:"data"`
}

type TxData struct {
	Hash        string `json:"hash"`
	From        string `json:"from"`
	To          string `json:"to"`
	Value       string `json:"value"`
	BlockNumber int64  `json:"blockNumber"`
	Timestamp   int64  `json:"timestamp"`
	Input       string `json:"input"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(StatusResponse{
			Data: []StatusData{
				{
					ClientBlock:  20000000,
					ScrapeBlock:  19500000,
					IndexedBlock: 19500000,
				},
			},
		})
	})

	http.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		addrs := r.URL.Query().Get("addrs")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ExportResponse{
			Data: []TxData{
				{
					Hash:        "0x" + uuid.New().String()[:32],
					From:        addrs,
					To:          "0x71c7656ec7ab88b098defb751b7401b5f6d8976f",
					Value:       "1000000000000000000",
					BlockNumber: 19499999,
					Timestamp:   1700000000,
					Input:       "0x",
				},
			},
		})
	})

	log.Printf("TrueBlocks local service listening on 0.0.0.0:%s", port)
	if err := http.ListenAndServe("0.0.0.0:"+port, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
