package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	"github.com/ishan662/user-service/pkg/userclient"
	"github.com/nats-io/nats.go"
	"github.com/go-playground/validator/v10"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var validate = validator.New()

func formatValidationError(err error) string {
	var sb strings.Builder
	for _, err := range err.(validator.ValidationErrors) {
		sb.WriteString(err.Field() + " is " + err.Tag() + "; ")
	}
	return sb.String()
}

func handleCorrections(client *userclient.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("Websocket upgrade failed", "error", err)
			return
		}
		defer ws.Close()

		updates := make(chan []byte, 10)
		sub, err := client.GetNATSConnection().Subscribe("user.*", func(m *nats.Msg) {
			updates <- m.Data
		})
		if err != nil {
			slog.Error("NATS subscribe failed", "error", err)
			return
		}
		defer sub.Unsubscribe()

		for {
			select {
			case msg := <-updates:
				if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-r.Context().Done():
				return
			}
		}
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("Starting gateway Service.")

	client, err := userclient.NewClient("nats://localhost:4222")
	if err != nil {
		slog.Error("Failed to connect to nats", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Get("/corrections", handleCorrections(client))

	r.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "id")

		response, err := client.GetUser(userID)

		if err != nil {
			slog.Error("User service request failed", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Write([]byte(response))
	})

	r.Post("/users", func(w http.ResponseWriter, r *http.Request) {
		var userReq struct {
			FirstName string `json:"first_name" validate:"required, min=2,max=50"`
			LastName  string `json:"last_name" validate:"required, min=2,max=50"`
			Email     string `json:"email" validate:"required,email"`
			Phone     string `json:"phone" validate:"omitempty,e164"`
			Age       int32  `json:"age" validate:"omitempty, gte=0, lte=150"`
		}

		if err := json.NewDecoder(r.Body).Decode(&userReq); err != nil {
			slog.Error("Failed to decode request ", "error", err)
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := validate.Struct(userReq); err != nil {
			slog.Warn("validation failed", "error", err)
			http.Error(w, "Validation Error: "+formatValidationError(err), http.StatusUnprocessableEntity)
			return
		}

		data, _ := json.Marshal(userReq)

		resp, err := client.CreateUser(data)
		if err != nil {
			slog.Error("User service create failed", "error", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}

		if strings.HasPrefix(string(resp), "Error:") {
			http.Error(w, string(resp), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(resp)
	})

	r.Delete("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "id")

		resp, err := client.DeleteUser(userID)
		if err != nil {
			slog.Error("User service delete failed", "user_id", userID, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if strings.HasPrefix(string(resp), "Error:") {
			slog.Warn("Failed to delete user", "user_id", userID, "response", string(resp))
			http.Error(w, string(resp), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	})

	slog.Info("Gateway is running on :8080")
	err = http.ListenAndServe(":8080", r)
	if err != nil {
		slog.Error("Server failed to start", "error", err)
	}
}
