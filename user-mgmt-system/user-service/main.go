package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/ishan662/user-service/internal/db"
	"github.com/nats-io/nats.go"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var validate = validator.New()

func formatValidationError(err error) string {
	var sb strings.Builder
	for _, err := range err.(validator.ValidationErrors) {
		sb.WriteString(err.Field() + " is " + err.Tag() + "; ")
	}
	return sb.String()
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("Starting User Service...")

	dbUrl := "postgres://user:password@localhost:5432/user_db?sslmode=disable"
	conn, err := sql.Open("pgx", dbUrl)
	if err != nil {
		slog.Error("Failed to open database connection", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := conn.Ping(); err != nil {
		slog.Error("Database unreachable", "error", err)
		os.Exit(1)
	}
	slog.Info("Successfully connected to PostgreSQL")

	queries := db.New(conn)

	nc, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		slog.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()
	slog.Info("Successfully connected to NATS")

	_, err = nc.Subscribe("user.get", func(m *nats.Msg) {
		userIDString := string(m.Data)
		slog.Info("Received RPC request", "user_id", userIDString)

		parsedID, err := uuid.Parse(userIDString)
		if err != nil {
			slog.Error("Invalid UUID received", "error", err)
			m.Respond([]byte("Error: Invalid UUID format"))
			return
		}

		user, err := queries.GetUser(context.Background(), parsedID)
		if err != nil {
			slog.Error("User not found in database", "user_id", parsedID, "error", err)
			m.Respond([]byte("Error: User not found"))
			return
		}

		userJSON, _ := json.Marshal(user)
		m.Respond(userJSON)
		
		slog.Info("Successfully sent user data back to Gateway", "user_id", parsedID)
	})

	_, err = nc.Subscribe("user.create", func(m *nats.Msg) {
		var req struct {
			FirstName string         `json:"first_name" validate:"required,min=2"`
			LastName  string         `json:"last_name" validate:"required,min=2"`
			Email     string         `json:"email" validate:"required,email"`
			Phone     *string        `json:"phone" validate:"omitempty,e164"`
			Age       *int32         `json:"age" validate:"omitempty,gte=0,lte=150"`
			Status    *db.UserStatus `json:"status" validate:"omitempty,oneof=Active Inactive"`
		}

		if err :=json.Unmarshal(m.Data, &req); err != nil {
			slog.Error("Failed to decode create request", "error", err)
			m.Respond([] byte("Error: Invalid Request Data"))
			return
		}

		if err :=validate.Struct(req); err != nil {
			slog.Warn("Validation Failed at user service boundry", "error", err)
			m.Respond([]byte("Error: Validation Failed - " + formatValidationError((err))))
			return
		}

		createParams := db.CreateUserParams{
			FirstName: req.FirstName,
			LastName:  req.LastName,
			Email:     req.Email,
		}

		if req.Phone != nil && *req.Phone != "" {
			createParams.Phone = sql.NullString{String: *req.Phone, Valid: true}
		}

		if req.Age != nil {
			createParams.Age = sql.NullInt32{Int32: *req.Age, Valid: true}
		}

		if req.Status == nil || *req.Status == "" {
			createParams.Status = db.UserStatusActive
		} else {
			createParams.Status = *req.Status
		}

		newUser, err := queries.CreateUser(context.Background(), createParams)
		if err != nil {
			slog.Error("Failed to save user to DB", "error", err)
			m.Respond([]byte("Error:Could not save user"))
			return 
		}

		resp, _ := json.Marshal(newUser)
		m.Respond(resp)

		err = nc.Publish("user.created", resp)
		if err != nil {
			slog.Error("Failed to publish user.created event", "error", err)
		}else{
			slog.Info("Event published: user.created", "user_id", newUser.UserID)
		}

		slog.Info("New user created successfully", "email", newUser.Email)
	})

	_, err = nc.Subscribe("user.delete", func(m *nats.Msg){
		userIdString := string(m.Data)
		parsedID, err := uuid.Parse(userIdString)
		if err != nil {
			m.Respond([]byte("Error: Invalid UUID"))
			return
		}

		err = queries.DeleteUser(context.Background(), parsedID)
		if err != nil {
			slog.Error("Failed to delete user", "error", err)
			m.Respond([]byte("Error: Could not delete user"))
			return
		}

		m.Respond([]byte(`{"message":"User deleted successfully"}`))

		nc.Publish("user.deleted", []byte(userIdString))
		slog.Info("User deleted", "user_id", parsedID)

	})

	slog.Info("User Service is fully operational")
	
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	
	<-quit 
	slog.Info("Shutting down User Service gracefully...")
}