package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync/atomic"

	"golang.org/x/net/websocket"
)

const (
	rtmURLformatter = "https://slack.com/api/rtm.start?token=%s"
	wsURL           = "https://api.slack.com/"
)

type SlackAdaptor struct {
	ws *websocket.Conn

	BotID   string
	counter uint64
}

type SlackMessage struct {
	ID      uint64 `json:"id"`
	Type    string `json:"type"`
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

func NewSlack(key string) (*SlackAdaptor, error) {
	url := fmt.Sprintf(rtmURLformatter, key)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Received %d while connecting to Slack (expected 200)", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	type Payload struct {
		Ok    bool
		Error string
		URL   string
		Self  struct {
			ID string
		}
	}
	var p Payload
	err = json.Unmarshal(body, &p)
	if err != nil {
		return nil, err
	}
	if !p.Ok {
		return nil, errors.New(p.Error)
	}

	ws, err := websocket.Dial(p.URL, "", wsURL)
	if err != nil {
		return nil, err
	}

	return &SlackAdaptor{ws: ws, BotID: p.Self.ID}, nil
}

func (a *SlackAdaptor) getSlackMessage() (*SlackMessage, error) {
	m := SlackMessage{}
	err := websocket.JSON.Receive(a.ws, &m)
	return &m, err
}

func (a *SlackAdaptor) Attach() (chan Message, chan error) {
	messagesCh := make(chan Message, 1)
	errorsCh := make(chan error)
	go func() {
		for {
			m, err := a.getSlackMessage()
			if err != nil {
				errorsCh <- err
				continue
			}
			if m.Type == "message" {
				messagesCh <- Message{Channel: m.Channel, Body: m.Text}
			}
		}
	}()
	return messagesCh, errorsCh
}

func (a *SlackAdaptor) Send(m Message) error {
	sm := SlackMessage{
		ID:      atomic.AddUint64(&a.counter, 1),
		Type:    "message",
		Channel: m.Channel,
		Text:    m.Body,
	}
	return websocket.JSON.Send(a.ws, sm)
}
