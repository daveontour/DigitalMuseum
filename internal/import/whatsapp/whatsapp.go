package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/daveontour/digitalmuseum/internal/import/utils"
	"github.com/daveontour/digitalmuseum/internal/importstorage"
)

var nonGroupChatNotificationPatterns = []string{
	"Messages to this chat and calls are now secured with end-to-end encryption",
	"You started a call", "You ended a call", "You joined a call", "You left a call",
	"You missed a call", "You rejected a call", "You accepted a call", "You declined a call",
	"You blocked a call", "changed their phone number to a new number", "is a contact",
	"This chat is now end-to-end encrypted", "Voice call -", "Video call -", "Missed video",
	"Missed voice", "This chat is with a business account", "turned on disappearing messages",
	"This business account has now registered as a standard account",
}

var cleanStringRegex = regexp.MustCompile(`[^\w\s]`)

// ImportStats holds statistics about the import process
type ImportStats struct {
	ConversationsProcessed         int
	TotalConversations             int
	MessagesImported               int
	MessagesUpdated                int
	MessagesCreated                int
	Errors                         int
	AttachmentsFound               int
	AttachmentsMissing             int
	AttachmentErrorsFileNotFound   int
	AttachmentErrorsFileRead       int
	AttachmentErrorsBlobInsert     int
	AttachmentErrorsMetadataInsert int
	AttachmentErrorsJunctionInsert int
	MissingAttachmentFilenames     []string
	AttachmentErrors               []string
	CurrentConversation            string
	mu                             sync.Mutex
}

func (s *ImportStats) copyStats() ImportStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ImportStats{
		ConversationsProcessed:         s.ConversationsProcessed,
		TotalConversations:             s.TotalConversations,
		MessagesImported:               s.MessagesImported,
		MessagesUpdated:                s.MessagesUpdated,
		MessagesCreated:                s.MessagesCreated,
		Errors:                         s.Errors,
		AttachmentsFound:               s.AttachmentsFound,
		AttachmentsMissing:             s.AttachmentsMissing,
		AttachmentErrorsFileNotFound:   s.AttachmentErrorsFileNotFound,
		AttachmentErrorsFileRead:       s.AttachmentErrorsFileRead,
		AttachmentErrorsBlobInsert:     s.AttachmentErrorsBlobInsert,
		AttachmentErrorsMetadataInsert: s.AttachmentErrorsMetadataInsert,
		AttachmentErrorsJunctionInsert: s.AttachmentErrorsJunctionInsert,
		MissingAttachmentFilenames:     append([]string(nil), s.MissingAttachmentFilenames...),
		AttachmentErrors:               append([]string(nil), s.AttachmentErrors...),
		CurrentConversation:            s.CurrentConversation,
	}
}

// ProgressCallback is called after each conversation is processed
type ProgressCallback func(ImportStats)

// CancelledCheck returns true if the import should be cancelled
type CancelledCheck func() bool

// ImportWhatsAppFromDirectory imports WhatsApp messages from a directory structure
func ImportWhatsAppFromDirectory(ctx context.Context, storage *importstorage.MessageStorage, directoryPath string, progressCallback ProgressCallback, checkFunc CancelledCheck) (*ImportStats, error) {
	cancelledCheck = checkFunc

	dirInfo, err := os.Stat(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("directory does not exist or is not accessible: %w", err)
	}
	if !dirInfo.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", directoryPath)
	}

	entries, err := os.ReadDir(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	totalConversations := 0
	for _, entry := range entries {
		if entry.IsDir() {
			totalConversations++
		}
	}

	stats := &ImportStats{
		TotalConversations:         totalConversations,
		MissingAttachmentFilenames: []string{},
		AttachmentErrors:           []string{},
	}

	var conversationDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			conversationDirs = append(conversationDirs, entry.Name())
		}
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > len(conversationDirs) {
		numWorkers = len(conversationDirs)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	conversationChan := make(chan string, len(conversationDirs))
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for conversationName := range conversationChan {
				if cancelledCheck != nil && cancelledCheck() {
					return
				}
				select {
				case <-ctx.Done():
					return
				default:
				}

				stats.mu.Lock()
				stats.ConversationsProcessed++
				stats.CurrentConversation = conversationName
				stats.mu.Unlock()

				subdirPath := filepath.Join(directoryPath, conversationName)

				csvFiles, err := filepath.Glob(filepath.Join(subdirPath, "*.csv"))
				if err != nil {
					slog.Error("finding CSV files", "conversation", conversationName, "err", err)
					stats.mu.Lock()
					stats.Errors++
					stats.mu.Unlock()
					continue
				}

				if len(csvFiles) == 0 {
					slog.Warn("no CSV file in subdirectory", "conversation", conversationName)
					if progressCallback != nil {
						progressCallback(stats.copyStats())
					}
					continue
				}

				for _, csvFile := range csvFiles {
					if cancelledCheck != nil && cancelledCheck() {
						return
					}
					select {
					case <-ctx.Done():
						return
					default:
					}

					err := processCSVFile(ctx, storage, csvFile, conversationName, stats)
					if err != nil {
						slog.Error("reading CSV file", "file", csvFile, "err", err)
						stats.mu.Lock()
						stats.Errors++
						stats.mu.Unlock()
						continue
					}
				}

				if progressCallback != nil {
					progressCallback(stats.copyStats())
				}
			}
		}()
	}

	for _, conversationName := range conversationDirs {
		conversationChan <- conversationName
	}
	close(conversationChan)

	wg.Wait()

	err = storage.SetIsGroupChat(ctx)
	if err != nil {
		slog.Warn("could not set is_group_chat flag", "err", err)
	}

	return stats, nil
}

var cancelledCheck CancelledCheck

func processCSVFile(ctx context.Context, storage *importstorage.MessageStorage, csvFilePath, conversationName string, stats *ImportStats) error {
	file, err := os.Open(csvFilePath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	messages, err := ParseCSV(file)
	if err != nil {
		return fmt.Errorf("failed to parse CSV: %w", err)
	}

	csvDir := filepath.Dir(csvFilePath)

	const batchSize = 100
	for i := 0; i < len(messages); i += batchSize {
		if cancelledCheck != nil && cancelledCheck() {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		end := i + batchSize
		if end > len(messages) {
			end = len(messages)
		}

		batch := messages[i:end]
		batchResults, err := processMessageBatch(ctx, storage, batch, csvDir, conversationName, stats)
		if err != nil {
			slog.Error("processing message batch", "err", err)
			stats.mu.Lock()
			stats.Errors += len(batch)
			stats.mu.Unlock()
			continue
		}

		stats.mu.Lock()
		stats.MessagesImported += batchResults.Created + batchResults.Updated
		stats.MessagesCreated += batchResults.Created
		stats.MessagesUpdated += batchResults.Updated
		stats.Errors += batchResults.Errors
		stats.AttachmentErrorsBlobInsert += batchResults.AttachmentErrorsBlobInsert
		stats.AttachmentErrorsMetadataInsert += batchResults.AttachmentErrorsMetadataInsert
		stats.AttachmentErrorsJunctionInsert += batchResults.AttachmentErrorsJunctionInsert
		stats.mu.Unlock()
	}

	return nil
}

func processMessageBatch(ctx context.Context, storage *importstorage.MessageStorage, messages []WhatsAppMessage, csvDir, conversationName string, stats *ImportStats) (*importstorage.BatchSaveResult, error) {
	batchMessages := make([]importstorage.MessageWithAttachment, 0, len(messages))

	for _, msg := range messages {
		messageData, attachmentData, attachmentFilename, attachmentType := prepareMessageData(msg, csvDir, conversationName, stats)

		batchMessages = append(batchMessages, importstorage.MessageWithAttachment{
			MessageData:        messageData,
			AttachmentData:     attachmentData,
			AttachmentFilename: attachmentFilename,
			AttachmentType:     attachmentType,
			Source:             "whatsapp",
		})
	}

	return storage.SaveMessagesBatch(ctx, batchMessages)
}

func prepareMessageData(msg WhatsAppMessage, csvDir, conversationName string, stats *ImportStats) (importstorage.MessageData, []byte, string, string) {
	chatSession := cleanString(msg.ChatSession)
	senderName := cleanString(msg.SenderName)

	messageData := importstorage.MessageData{
		ChatSession:   stringPtr(chatSession),
		MessageDate:   msg.MessageDate,
		DeliveredDate: msg.SentDate,
		ReadDate:      nil,
		EditedDate:    nil,
		Service:       stringPtr("WhatsApp"),
		Type:          stringPtr(msg.Type),
		SenderID:      stringPtr(msg.SenderID),
		SenderName:    stringPtr(senderName),
		Status:        stringPtr(msg.Status),
		ReplyingTo:    stringPtr(msg.ReplyingTo),
		Subject:       nil,
		Text:          stringPtr(msg.Text),
		IsGroupChat:   false,
	}

	if msg.Type == "Notification" && msg.Text != "" {
		isGroupChat := true
		for _, pattern := range nonGroupChatNotificationPatterns {
			if strings.Contains(msg.Text, pattern) {
				isGroupChat = false
				break
			}
		}
		messageData.IsGroupChat = isGroupChat
	}

	var attachmentData []byte
	attachmentFilename := ""
	attachmentType := ""

	if msg.Attachment != "" {
		attachmentFilename = msg.Attachment
		attachmentType = msg.AttachmentType

		filePath, actualFilename, err := utils.FindAttachmentFileWithFallback(csvDir, attachmentFilename)
		if err == nil {
			data, err := os.ReadFile(filePath)
			if err == nil {
				attachmentData = data
				stats.mu.Lock()
				stats.AttachmentsFound++
				stats.mu.Unlock()

				if actualFilename != attachmentFilename {
					attachmentFilename = actualFilename
					if strings.HasSuffix(strings.ToLower(actualFilename), ".jpg") {
						attachmentType = "image/jpeg"
					} else if strings.HasSuffix(strings.ToLower(actualFilename), ".mp3") {
						attachmentType = "audio/mpeg"
					}
				}

				attachmentType = utils.NormalizeMIMEType(attachmentType, attachmentFilename)
			} else {
				var fileSize int64 = -1
				if fileInfo, statErr := os.Stat(filePath); statErr == nil {
					fileSize = fileInfo.Size()
				}
				errorMsg := fmt.Sprintf("Conversation: %s | Attachment: %s | File path: %s | Size: %d bytes | Error: %v",
					conversationName, attachmentFilename, filePath, fileSize, err)
				slog.Error("could not read attachment file", "msg", errorMsg)
				stats.mu.Lock()
				stats.AttachmentErrorsFileRead++
				if !contains(stats.AttachmentErrors, errorMsg) {
					stats.AttachmentErrors = append(stats.AttachmentErrors, errorMsg)
				}
				missingFilename := fmt.Sprintf("%s/%s", conversationName, attachmentFilename)
				stats.AttachmentsMissing++
				if !contains(stats.MissingAttachmentFilenames, missingFilename) {
					stats.MissingAttachmentFilenames = append(stats.MissingAttachmentFilenames, missingFilename)
				}
				stats.mu.Unlock()
			}
		} else {
			errorMsg := fmt.Sprintf("Conversation: %s | Attachment: %s | Searched in: %s | Error: %v",
				conversationName, attachmentFilename, csvDir, err)
			slog.Warn("attachment file not found", "msg", errorMsg)
			stats.mu.Lock()
			stats.AttachmentErrorsFileNotFound++
			stats.AttachmentsMissing++
			missingFilename := fmt.Sprintf("%s/%s", conversationName, attachmentFilename)
			if !contains(stats.MissingAttachmentFilenames, missingFilename) {
				stats.MissingAttachmentFilenames = append(stats.MissingAttachmentFilenames, missingFilename)
			}
			stats.mu.Unlock()
		}
	}

	return messageData, attachmentData, attachmentFilename, attachmentType
}

func cleanString(s string) string {
	if s == "" {
		return ""
	}
	return strings.TrimSpace(cleanStringRegex.ReplaceAllString(s, ""))
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
