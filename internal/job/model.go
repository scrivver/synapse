package job

import (
	"encoding/json"

	"github.com/google/uuid"
)

type Type string

const (
	TypeMoveFile Type = "move_file"
)

type Job struct {
	ID      string  `json:"job_id"`
	Type    Type    `json:"type"`
	Payload Payload `json:"payload"`
	Retry   int     `json:"retry"`
}

type Payload struct {
	FileID string `json:"file_id"`
	From   string `json:"from"`
	To     string `json:"to"`
}

func NewMoveFileJob(fileID, from, to string) Job {
	return Job{
		ID:   uuid.New().String(),
		Type: TypeMoveFile,
		Payload: Payload{
			FileID: fileID,
			From:   from,
			To:     to,
		},
	}
}

func Marshal(j Job) ([]byte, error) {
	return json.Marshal(j)
}

func Unmarshal(data []byte) (Job, error) {
	var j Job
	err := json.Unmarshal(data, &j)
	return j, err
}
