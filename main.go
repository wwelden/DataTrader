package main

import (
	"backend/handlers"
	"backend/middleware"
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

func main() {
	// run holds the deferred cleanup, so it always happens before we exit.
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// .env is optional; in production, config comes from real environment variables.
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return err
	}

	InitDB()
	defer db.Close()

	handlers.SetDB(db)

	middleware.StartSessionCleanup()
	handlers.StartDemoCleanup()

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
	router.Post("/demo", handlers.HandleDemoPost)

	// Public: landing page for visitors, dashboard for logged-in users.
	router.Get("/", handlers.HandleRoot)

	router.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)

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
		r.Get("/api/positions/filter", handlers.HandlePositionsFilter)

		r.Get("/api/positions/edit-stock/{id}", handlers.HandleEditStockPosition)
		r.Post("/api/positions/update-stock/{id}", handlers.HandleUpdateStockPosition)
		r.Get("/api/positions/edit-option/{id}", handlers.HandleEditOptionPosition)
		r.Post("/api/positions/update-option/{id}", handlers.HandleUpdateOptionPosition)

		r.Delete("/api/positions/stock/{id}", handlers.HandleDeleteStockPosition)
		r.Delete("/api/positions/option/{id}", handlers.HandleDeleteOptionPosition)

		r.Post("/api/positions/close/{id}", handlers.HandleClosePosition)
		r.Post("/api/positions/close-option-modal/{id}", handlers.HandleCloseOptionModal)
		r.Get("/api/positions/outcome-fields/{id}", handlers.HandleOutcomeFields)
		r.Post("/api/positions/close-stock/{id}", handlers.HandleCloseStockPosition)
		r.Post("/api/positions/close-option/{id}", handlers.HandleCloseOptionPosition)

		r.Get("/api/history/stocks", handlers.HandleGetClosedStocks)
		r.Get("/api/history/options", handlers.HandleGetClosedOptions)
		r.Get("/api/history/filter", handlers.HandleHistoryFilter)

		r.Get("/api/history/edit-stock/{id}", handlers.HandleEditClosedStock)
		r.Post("/api/history/update-stock/{id}", handlers.HandleUpdateClosedStock)
		r.Get("/api/history/edit-option/{id}", handlers.HandleEditClosedOption)
		r.Post("/api/history/update-option/{id}", handlers.HandleUpdateClosedOption)

		r.Delete("/api/history/stock/{id}", handlers.HandleDeleteClosedStock)
		r.Delete("/api/history/option/{id}", handlers.HandleDeleteClosedOption)

		r.Post("/api/import-csv", handlers.HandleImportCSV)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = ":8080"
	}
	if port[0] != ':' {
		port = ":" + port
	}
	// Stop cleanly when Railway (or Ctrl-C) asks the process to exit, so a
	// redeploy is not reported as a crash and the database is closed properly.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	ln, err := net.Listen("tcp", port)
	if err != nil {
		return err
	}
	slog.Info("HTTP server started", "listenAddr", port)
	err = runServer(ctx, ln, router, shutdownTimeout)
	if errors.Is(err, errShutdownTimedOut) {
		// We were asked to stop and did; a slow request was cut off. That is
		// worth a warning, not a failed exit.
		slog.Warn("HTTP server stopped before all requests finished", "err", err)
		return nil
	}
	if err != nil {
		return err
	}
	slog.Info("HTTP server stopped")
	return nil
}
