package userclient

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

type Client struct {
	nc    *nats.Conn
	cache map[string][]byte
	mu    sync.RWMutex
}

func NewClient(natsURL string) (*Client, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, err
	}

	c := &Client{
		nc:    nc,
		cache: make(map[string][]byte),
	}

	c.listenForEvents()

	return c, nil
}

func (c *Client) GetUser(userID string) (string, error) {
	c.mu.RLock()
	if data, found := c.cache[userID]; found {
		c.mu.RUnlock()
		return string(data), nil
	}
	c.mu.RUnlock()

	msg, err := c.nc.Request("user.get", []byte(userID), 2*time.Second)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(string(msg.Data), "Error:") {
		c.mu.Lock()
		c.cache[userID] = msg.Data
		c.mu.Unlock()
	}

	return string(msg.Data), nil
}

func (c *Client) CreateUser(data []byte) ([]byte, error) {
	msg, err := c.nc.Request("user.create", data, 2*time.Second)
	if err != nil {
		return nil, err
	}
	return msg.Data, nil
}

func (c *Client) DeleteUser(userID string) ([]byte, error) {
	msg, err := c.nc.Request("user.delete", []byte(userID), 2*time.Second)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(string(msg.Data), "Error:") {
		c.mu.Lock()
		delete(c.cache, userID)
		c.mu.Unlock()
	}

	return msg.Data, nil
}

func (c *Client) listenForEvents() {
	c.nc.Subscribe("user.deleted", func(m *nats.Msg) {
		userID := string(m.Data)
		c.mu.Lock()
		delete(c.cache, userID)
		c.mu.Unlock()
	})

	c.nc.Subscribe("user.updated", func(m *nats.Msg) {
		var updatedUser struct {
			UserID string `json:"user_id"`
		}
		if err := json.Unmarshal(m.Data, &updatedUser); err == nil {
			c.mu.Lock()
			delete(c.cache, updatedUser.UserID)
			c.mu.Unlock()
		}
	})
}
func (c *Client) GetNATSConnection() *nats.Conn {
	return c.nc
}
func (c *Client) Close() {
	if c.nc != nil {
		c.nc.Close()
	}
}