package main

import (
	"backend/handlers"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

func main() {
	InitDB()
	defer db.Close()

	handlers.SetDB(db)

	if err := godotenv.Load(); err != nil {
		log.Fatal(err)
	}
	router := chi.NewMux()

	// Serve CSS with correct MIME type
	router.Get("/public/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		http.ServeFile(w, r, "public/style.css")
	})

	router.Handle("/public/*", public())

	// Page routes
	router.Get("/", handlers.HandleHome)
	router.Get("/positions.html", handlers.HandlePositions)
	router.Get("/history.html", handlers.HandleHistory)

	// Modal routes
	router.Get("/modal/add-position.html", handlers.HandleModalAddPosition)
	router.Get("/modal/add-position-fields.html", handlers.HandleModalAddPositionFields)
	router.Get("/modal/close", handlers.HandleModalClose)

	// API routes
	router.Get("/api/stats", handlers.HandleStats)
	router.Post("/api/positions/add", handlers.HandleAddPosition)
	router.Get("/api/positions/stocks", handlers.HandleGetStockPositions)
	router.Get("/api/positions/options", handlers.HandleGetOptionPositions)
	router.Post("/api/positions/close/{id}", handlers.HandleClosePosition)
	router.Post("/api/positions/close-stock/{id}", handlers.HandleCloseStockPosition)
	router.Post("/api/positions/close-option/{id}", handlers.HandleCloseOptionPosition)
	router.Get("/api/history/stocks", handlers.HandleGetClosedStocks)
	router.Get("/api/history/options", handlers.HandleGetClosedOptions)

	port := os.Getenv("PORT")
	slog.Info("HTTP server started", "listenAddr", port)
	http.ListenAndServe(port, router)
}
