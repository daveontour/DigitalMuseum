package imessage

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"
)

// IMessageMessage represents a parsed iMessage from CSV (iMazing format)
type IMessageMessage struct {
	MessageDate    *time.Time
	DeliveredDate  *time.Time
	ReadDate       *time.Time
	EditedDate     *time.Time
	ChatSession    string
	Service        string
	Type           string
	SenderID       string
	SenderName     string
	Status         string
	ReplyingTo     string
	Subject        string
	Text           string
	Attachment     string
	AttachmentType string
}

func parseDate(dateStr string) (*time.Time, error) {
	if dateStr == "" {
		return nil, nil
	}
	dateStr = trimSpace(dateStr)
	if dateStr == "" {
		return nil, nil
	}
	layout := "2006-01-02 15:04:05"
	t, err := time.Parse(layout, dateStr)
	if err != nil {
		return nil, fmt.Errorf("could not parse date '%s': %w", dateStr, err)
	}
	return &t, nil
}

// ParseIMessageCSV parses an iMessage CSV file and returns messages
func ParseIMessageCSV(reader io.Reader) ([]IMessageMessage, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1

	header, err := csvReader.Read()
	if err == io.EOF {
		return []IMessageMessage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	columnMap := make(map[string]int)
	for i, col := range header {
		columnMap[trimSpace(col)] = i
	}

	var messages []IMessageMessage

	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CSV row: %w", err)
		}

		msg := IMessageMessage{}

		if idx, ok := columnMap["Message Date"]; ok && idx < len(row) {
			if date, err := parseDate(row[idx]); err == nil {
				msg.MessageDate = date
			}
		}
		if idx, ok := columnMap["Delivered Date"]; ok && idx < len(row) {
			if date, err := parseDate(row[idx]); err == nil {
				msg.DeliveredDate = date
			}
		}
		if idx, ok := columnMap["Read Date"]; ok && idx < len(row) {
			if date, err := parseDate(row[idx]); err == nil {
				msg.ReadDate = date
			}
		}
		if idx, ok := columnMap["Edited Date"]; ok && idx < len(row) {
			if date, err := parseDate(row[idx]); err == nil {
				msg.EditedDate = date
			}
		}
		if idx, ok := columnMap["Chat Session"]; ok && idx < len(row) {
			msg.ChatSession = trimSpace(row[idx])
		}
		if idx, ok := columnMap["Service"]; ok && idx < len(row) {
			msg.Service = trimSpace(row[idx])
		}
		if idx, ok := columnMap["Type"]; ok && idx < len(row) {
			msg.Type = trimSpace(row[idx])
		}
		if idx, ok := columnMap["Sender ID"]; ok && idx < len(row) {
			msg.SenderID = trimSpace(row[idx])
		}
		if idx, ok := columnMap["Sender Name"]; ok && idx < len(row) {
			msg.SenderName = trimSpace(row[idx])
		}
		if idx, ok := columnMap["Status"]; ok && idx < len(row) {
			msg.Status = trimSpace(row[idx])
		}
		if idx, ok := columnMap["Replying to"]; ok && idx < len(row) {
			msg.ReplyingTo = trimSpace(row[idx])
		}
		if idx, ok := columnMap["Subject"]; ok && idx < len(row) {
			msg.Subject = trimSpace(row[idx])
		}
		if idx, ok := columnMap["Text"]; ok && idx < len(row) {
			msg.Text = trimSpace(row[idx])
		}
		if idx, ok := columnMap["Attachment"]; ok && idx < len(row) {
			msg.Attachment = trimSpace(row[idx])
		}
		if idx, ok := columnMap["Attachment type"]; ok && idx < len(row) {
			msg.AttachmentType = trimSpace(row[idx])
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
