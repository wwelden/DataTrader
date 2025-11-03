package main

import (
	"backend/handlers"
	"backend/middleware"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal(err)
	}

	InitDB()
	defer db.Close()

	handlers.SetDB(db)

	middleware.StartSessionCleanup()

	router := chi.NewMux()

	router.Get("/public/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		http.ServeFile(w, r, "public/style.css")
	})

	router.Handle("/public/*", public())

	router.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	router.Get("/login", handlers.HandleLogin)
	router.Post("/api/auth/login", handlers.HandleLoginPost)
	router.Get("/signup", handlers.HandleSignup)
	router.Post("/api/auth/signup", handlers.HandleSignupPost)
	router.Post("/api/logout", handlers.HandleLogout)

	router.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)

		r.Get("/", handlers.HandleHome)
		r.Get("/positions.html", handlers.HandlePositions)
		r.Get("/history.html", handlers.HandleHistory)

		r.Get("/modal/add-position.html", handlers.HandleModalAddPosition)
		r.Get("/modal/add-position-fields.html", handlers.HandleModalAddPositionFields)
		r.Get("/modal/import-csv.html", handlers.HandleModalImportCSV)
		r.Get("/modal/close", handlers.HandleModalClose)

		r.Get("/api/stats", handlers.HandleStats)
		r.Post("/api/positions/add", handlers.HandleAddPosition)
		r.Get("/api/positions/stocks", handlers.HandleGetStockPositions)
		r.Get("/api/positions/options", handlers.HandleGetOptionPositions)

		r.Get("/api/positions/edit-stock/{id}", handlers.HandleEditStockPosition)
		r.Post("/api/positions/update-stock/{id}", handlers.HandleUpdateStockPosition)
		r.Get("/api/positions/edit-option/{id}", handlers.HandleEditOptionPosition)
		r.Post("/api/positions/update-option/{id}", handlers.HandleUpdateOptionPosition)

		r.Delete("/api/positions/stock/{id}", handlers.HandleDeleteStockPosition)
		r.Delete("/api/positions/option/{id}", handlers.HandleDeleteOptionPosition)

		r.Post("/api/positions/close/{id}", handlers.HandleClosePosition)
		r.Post("/api/positions/close-option-modal/{id}", handlers.HandleCloseOptionModal)
		r.Post("/api/positions/close-stock/{id}", handlers.HandleCloseStockPosition)
		r.Post("/api/positions/close-option/{id}", handlers.HandleCloseOptionPosition)

		r.Get("/api/history/stocks", handlers.HandleGetClosedStocks)
		r.Get("/api/history/options", handlers.HandleGetClosedOptions)

		r.Get("/api/history/edit-stock/{id}", handlers.HandleEditClosedStock)
		r.Post("/api/history/update-stock/{id}", handlers.HandleUpdateClosedStock)
		r.Get("/api/history/edit-option/{id}", handlers.HandleEditClosedOption)
		r.Post("/api/history/update-option/{id}", handlers.HandleUpdateClosedOption)

		r.Delete("/api/history/stock/{id}", handlers.HandleDeleteClosedStock)
		r.Delete("/api/history/option/{id}", handlers.HandleDeleteClosedOption)

		r.Post("/api/import-csv", handlers.HandleImportCSV)
	})

	port := os.Getenv("PORT")
	slog.Info("HTTP server started", "listenAddr", port)
	http.ListenAndServe(port, router)
}
