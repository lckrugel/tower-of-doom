package game

type MessageType string

const (
	InfoMessage   MessageType = "info"
	CombatMessage MessageType = "combat"
	StatusMessage MessageType = "status"
	ErrorMessage  MessageType = "error"
)

type GameLog struct {
	Messages []*GameMessage `json:"messages"`
}

func NewGameLog() *GameLog {
	return &GameLog{Messages: make([]*GameMessage, 0)}
}

func (gl *GameLog) AddNewMessage(msgType MessageType, content string, data map[string]any) {
	msg := &GameMessage{
		Type:    msgType,
		Content: content,
	}
	if len(data) > 0 {
		msg.Data = data
	}
	gl.Messages = append(gl.Messages, msg)
}

func (gl *GameLog) AddMessages(msgs ...*GameMessage) {
	gl.Messages = append(gl.Messages, msgs...)
}

func (gl *GameLog) ToStrings() []string {
	result := make([]string, 0, len(gl.Messages))
	for _, msg := range gl.Messages {
		result = append(result, msg.Content)
	}
	return result
}

type GameMessage struct {
	Type    MessageType    `json:"type"`
	Content string         `json:"content"`
	Data    map[string]any `json:"data,omitempty"`
}
