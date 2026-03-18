package whatsapp

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"
)

// WhatsAppMessage represents a parsed WhatsApp message from CSV
type WhatsAppMessage struct {
	MessageDate    *time.Time
	SentDate       *time.Time
	ChatSession    string
	Type           string
	SenderID       string
	SenderName     string
	Status         string
	ReplyingTo     string
	Text           string
	Attachment     string
	AttachmentType string
}

// ParseDate parses a date string from CSV format to time.Time
func ParseDate(dateStr string) (*time.Time, error) {
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

// ParseCSV parses a WhatsApp CSV file and returns messages
func ParseCSV(reader io.Reader) ([]WhatsAppMessage, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1

	header, err := csvReader.Read()
	if err == io.EOF {
		return []WhatsAppMessage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	columnMap := make(map[string]int)
	for i, col := range header {
		columnMap[trimSpace(col)] = i
	}

	var messages []WhatsAppMessage

	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CSV row: %w", err)
		}

		msg := WhatsAppMessage{}

		if idx, ok := columnMap["Message Date"]; ok && idx < len(row) {
			if date, err := ParseDate(row[idx]); err == nil {
				msg.MessageDate = date
			}
		}
		if idx, ok := columnMap["Sent Date"]; ok && idx < len(row) {
			if date, err := ParseDate(row[idx]); err == nil {
				msg.SentDate = date
			}
		}
		if idx, ok := columnMap["Chat Session"]; ok && idx < len(row) {
			msg.ChatSession = trimSpace(row[idx])
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
