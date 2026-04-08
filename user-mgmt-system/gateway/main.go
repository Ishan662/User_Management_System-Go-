package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/websocket"
	"github.com/ishan662/user-service/pkg/userclient"
	"github.com/nats-io/nats.go"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSMessage struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

var validate = validator.New()

func formatValidationError(err error) string {
	var sb strings.Builder
	if ve, ok := err.(validator.ValidationErrors); ok {
		for _, e := range ve {
			sb.WriteString(e.Field() + " is " + e.Tag() + "; ")
		}
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

		slog.Info("WebSocket client connected")

		updates := make(chan []byte, 10)
		var sub *nats.Subscription
		var mu sync.Mutex

		cleanup := func() {
			if sub != nil {
				sub.Unsubscribe()
				sub = nil
				slog.Info("NATS subscription cleaned up")
			}
		}
		defer cleanup()

		go func() {
			for {
				_, message, err := ws.ReadMessage()
				if err != nil {
					slog.Info("WebSocket client disconnected")
					return
				}

				var wsReq WSMessage
				if err := json.Unmarshal(message, &wsReq); err != nil {
					slog.Error("Invalid WS message format", "error", err)
					mu.Lock()
					ws.WriteJSON(map[string]string{"error": "invalid message format"})
					mu.Unlock()
					continue
				}

				switch strings.ToUpper(wsReq.Action) {
				case "SUBSCRIBE":
					if sub == nil {
						sub, _ = client.GetNATSConnection().Subscribe("user.*", func(m *nats.Msg) {
							updates <- m.Data
						})
						slog.Info("Client subscribed to real-time updates via WS")
					}
				case "CREATE":
					resp, err := client.CreateUser(wsReq.Payload)
					mu.Lock()
					if err != nil {
						ws.WriteJSON(map[string]string{"error": err.Error()})
					} else {
						ws.WriteMessage(websocket.TextMessage, resp)
					}
					mu.Unlock()
				case "DELETE":
					userID := strings.Trim(string(wsReq.Payload), "\"")
					resp, err := client.DeleteUser(userID)
					mu.Lock()
					if err != nil {
						ws.WriteJSON(map[string]string{"error": err.Error()})
					} else {
						ws.WriteMessage(websocket.TextMessage, resp)
					}
					mu.Unlock()
				default:
					mu.Lock()
					ws.WriteJSON(map[string]string{"error": "unknown action"})
					mu.Unlock()
				}
			}
		}()

		for {
			select {
			case msg := <-updates:
				mu.Lock()
				if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
					mu.Unlock()
					return
				}
				mu.Unlock()
			case <-r.Context().Done():
				return
			}
		}
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("Starting Gateway Service")
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	client, err := userclient.NewClient(natsURL)
	if err != nil {
		slog.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/corrections", handleCorrections(client))

	r.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "id")
		response, err := client.GetUser(userID)
		if err != nil {
			slog.Error("User request failed", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	})

	r.Post("/users", func(w http.ResponseWriter, r *http.Request) {
		var userReq struct {
			FirstName string `json:"first_name" validate:"required,min=2,max=50"`
			LastName  string `json:"last_name" validate:"required,min=2,max=50"`
			Email     string `json:"email" validate:"required,email"`
			Phone     string `json:"phone" validate:"omitempty,e164"`
			Age       int32  `json:"age" validate:"omitempty,gte=0,lte=150"`
		}

		if err := json.NewDecoder(r.Body).Decode(&userReq); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := validate.Struct(userReq); err != nil {
			http.Error(w, "Validation Error: "+formatValidationError(err), http.StatusUnprocessableEntity)
			return
		}

		data, _ := json.Marshal(userReq)
		resp, err := client.CreateUser(data)
		if err != nil {
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
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if strings.HasPrefix(string(resp), "Error:") {
			http.Error(w, string(resp), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(resp)
	})

	port := ":8080"
	slog.Info("Gateway is running", "port", port)
	if err := http.ListenAndServe(port, r); err != nil {
		slog.Error("Server failed", "error", err)
	}
}