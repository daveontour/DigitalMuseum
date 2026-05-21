package imessage

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
	"time"

	"github.com/daveontour/aimuseum/internal/import/utils"
	"github.com/daveontour/aimuseum/internal/importstorage"
)

var cleanStringRegex = regexp.MustCompile(`[^\w\s]`)

// folderAggregate collects metadata across all CSV files in one conversation folder.
type folderAggregate struct {
	maxMessageDate  *time.Time
	lastChatSession string
	lastService     string
}

func (a *folderAggregate) ingestMessages(messages []IMessageMessage) {
	if a == nil {
		return
	}
	for _, m := range messages {
		if m.MessageDate != nil {
			if a.maxMessageDate == nil || m.MessageDate.After(*a.maxMessageDate) {
				t := *m.MessageDate
				a.maxMessageDate = &t
			}
		}
		if strings.TrimSpace(m.ChatSession) != "" {
			a.lastChatSession = strings.TrimSpace(m.ChatSession)
		}
		if strings.TrimSpace(m.Service) != "" {
			a.lastService = strings.TrimSpace(m.Service)
		}
	}
}

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
	PlaceholdersUsed               int
	OrphanAttachmentsImported      int
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
		PlaceholdersUsed:               s.PlaceholdersUsed,
		OrphanAttachmentsImported:      s.OrphanAttachmentsImported,
		CurrentConversation:            s.CurrentConversation,
	}
}

// ProgressCallback is called after each conversation is processed
type ProgressCallback func(ImportStats)

// CancelledCheck returns true if the import should be cancelled
type CancelledCheck func() bool

var imessageCancelledCheck CancelledCheck

// ImportIMessagesFromDirectory imports iMessage conversations from a directory structure
func ImportIMessagesFromDirectory(ctx context.Context, storage *importstorage.MessageStorage, directoryPath string, progressCallback ProgressCallback, checkFunc CancelledCheck) (*ImportStats, error) {
	imessageCancelledCheck = checkFunc

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

	subjectFullName := storage.GetSubjectFullName()
	var subjectFullNamePtr *string
	if subjectFullName != "" {
		subjectFullNamePtr = &subjectFullName
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
				if imessageCancelledCheck != nil && imessageCancelledCheck() {
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

				usedBasenames := make(map[string]struct{})
				folderAgg := &folderAggregate{}

				for _, csvFile := range csvFiles {
					if imessageCancelledCheck != nil && imessageCancelledCheck() {
						return
					}
					select {
					case <-ctx.Done():
						return
					default:
					}

					err := processIMessageCSVFile(ctx, storage, csvFile, conversationName, stats, subjectFullNamePtr, usedBasenames, folderAgg)
					if err != nil {
						slog.Error("reading CSV file", "file", csvFile, "err", err)
						stats.mu.Lock()
						stats.Errors++
						stats.mu.Unlock()
						continue
					}
				}

				if err := appendOrphanFolderImages(ctx, storage, subdirPath, conversationName, stats, usedBasenames, folderAgg); err != nil {
					slog.Error("orphan folder images", "conversation", conversationName, "err", err)
					stats.mu.Lock()
					stats.Errors++
					stats.mu.Unlock()
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

	return stats, nil
}

func processIMessageCSVFile(ctx context.Context, storage *importstorage.MessageStorage, csvFilePath, conversationName string, stats *ImportStats, subjectFullName *string, usedBasenames map[string]struct{}, folderAgg *folderAggregate) error {
	file, err := os.Open(csvFilePath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	messages, err := ParseIMessageCSV(file)
	if err != nil {
		return fmt.Errorf("failed to parse CSV: %w", err)
	}

	folderAgg.ingestMessages(messages)

	csvDir := filepath.Dir(csvFilePath)

	const batchSize = 100
	for i := 0; i < len(messages); i += batchSize {
		if imessageCancelledCheck != nil && imessageCancelledCheck() {
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
		batchResults, err := processIMessageBatch(ctx, storage, batch, csvDir, conversationName, stats, subjectFullName, usedBasenames)
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

func appendOrphanFolderImages(ctx context.Context, storage *importstorage.MessageStorage, subdirPath, conversationName string, stats *ImportStats, usedBasenames map[string]struct{}, folderAgg *folderAggregate) error {
	if imessageCancelledCheck != nil && imessageCancelledCheck() {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	allImages, err := ListImageBasenames(subdirPath)
	if err != nil {
		return fmt.Errorf("list image basenames: %w", err)
	}

	var orphans []string
	for _, base := range allImages {
		if _, used := usedBasenames[base]; !used {
			orphans = append(orphans, base)
		}
	}
	if len(orphans) == 0 {
		return nil
	}

	chatSession := cleanString(folderAgg.lastChatSession)
	if chatSession == "" {
		chatSession = cleanString(conversationName)
	}

	service := folderAgg.lastService
	if service == "" {
		service = "iMessage"
	}
	src := attachmentSourceFromService(service)

	baseTime := time.Now().UTC().Truncate(time.Second)
	if folderAgg.maxMessageDate != nil {
		baseTime = folderAgg.maxMessageDate.UTC()
	} else if fi, err := os.Stat(subdirPath); err == nil {
		baseTime = fi.ModTime().UTC().Truncate(time.Second)
	}

	const orphanSender = "iMazing-orphan"
	orphanText := "Imported from device folder; not linked in CSV"

	var batch []importstorage.MessageWithAttachment
	for k, base := range orphans {
		if imessageCancelledCheck != nil && imessageCancelledCheck() {
			return ctx.Err()
		}
		path := filepath.Join(subdirPath, base)
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("orphan image read failed", "path", path, "err", err)
			continue
		}
		mt := utils.NormalizeMIMEType("", base)
		msgDate := baseTime.Add(time.Duration(k+1) * time.Second)
		batch = append(batch, importstorage.MessageWithAttachment{
			MessageData: importstorage.MessageData{
				ChatSession:   stringPtr(chatSession),
				MessageDate:   &msgDate,
				DeliveredDate: &msgDate,
				ReadDate:      nil,
				EditedDate:    nil,
				Service:       stringPtr(service),
				Type:          stringPtr("Incoming"),
				SenderID:      stringPtr(orphanSender),
				SenderName:    stringPtr(orphanSender),
				Status:        stringPtr("Received"),
				ReplyingTo:    nil,
				Subject:       nil,
				Text:          stringPtr(orphanText),
				IsGroupChat:   false,
			},
			AttachmentData:     data,
			AttachmentFilename: base,
			AttachmentType:     mt,
			Source:             src,
		})
	}

	if len(batch) == 0 {
		return nil
	}

	res, err := storage.SaveMessagesBatch(ctx, batch)
	if err != nil {
		return err
	}

	// One attachment per orphan row; count messages actually persisted (not validation skips).
	orphanOK := res.Created + res.Updated
	stats.mu.Lock()
	stats.MessagesImported += res.Created + res.Updated
	stats.MessagesCreated += res.Created
	stats.MessagesUpdated += res.Updated
	stats.Errors += res.Errors
	stats.AttachmentsFound += orphanOK
	stats.OrphanAttachmentsImported += orphanOK
	stats.AttachmentErrorsBlobInsert += res.AttachmentErrorsBlobInsert
	stats.AttachmentErrorsMetadataInsert += res.AttachmentErrorsMetadataInsert
	stats.AttachmentErrorsJunctionInsert += res.AttachmentErrorsJunctionInsert
	stats.mu.Unlock()

	return nil
}

// mediaItemSource maps the CSV Service column to media_items.source (SMS/MMS vs iMessage).
func mediaItemSource(msg IMessageMessage) string {
	return attachmentSourceFromService(msg.Service)
}

func attachmentSourceFromService(service string) string {
	s := strings.TrimSpace(service)
	if strings.EqualFold(s, "SMS") || strings.EqualFold(s, "MMS") {
		return "sms"
	}
	return "imessage"
}

func processIMessageBatch(ctx context.Context, storage *importstorage.MessageStorage, messages []IMessageMessage, csvDir, conversationName string, stats *ImportStats, subjectFullName *string, usedBasenames map[string]struct{}) (*importstorage.BatchSaveResult, error) {
	batchMessages := make([]importstorage.MessageWithAttachment, 0, len(messages))

	for _, msg := range messages {
		messageData, attachmentData, attachmentFilename, attachmentType := prepareIMessageData(msg, csvDir, conversationName, stats, subjectFullName, usedBasenames)

		batchMessages = append(batchMessages, importstorage.MessageWithAttachment{
			MessageData:        messageData,
			AttachmentData:     attachmentData,
			AttachmentFilename: attachmentFilename,
			AttachmentType:     attachmentType,
			Source:             mediaItemSource(msg),
		})
	}

	return storage.SaveMessagesBatch(ctx, batchMessages)
}

func prepareIMessageData(msg IMessageMessage, csvDir, conversationName string, stats *ImportStats, subjectFullName *string, usedBasenames map[string]struct{}) (importstorage.MessageData, []byte, string, string) {
	chatSession := cleanString(msg.ChatSession)
	senderName := cleanString(msg.SenderName)

	if msg.Type == "Outgoing" && senderName == "" && subjectFullName != nil && *subjectFullName != "" {
		senderName = *subjectFullName
	}

	service := msg.Service
	if service == "" {
		service = "iMessage"
	}

	messageData := importstorage.MessageData{
		ChatSession:   stringPtr(chatSession),
		MessageDate:   msg.MessageDate,
		DeliveredDate: msg.DeliveredDate,
		ReadDate:      msg.ReadDate,
		EditedDate:    msg.EditedDate,
		Service:       stringPtr(service),
		Type:          stringPtr(msg.Type),
		SenderID:      stringPtr(msg.SenderID),
		SenderName:    stringPtr(senderName),
		Status:        stringPtr(msg.Status),
		ReplyingTo:    stringPtr(msg.ReplyingTo),
		Subject:       stringPtr(msg.Subject),
		Text:          stringPtr(msg.Text),
		IsGroupChat:   false,
	}

	var attachmentData []byte
	attachmentFilename := ""
	attachmentType := ""

	if msg.Attachment != "" {
		attachmentFilename = msg.Attachment
		attachmentType = msg.AttachmentType

		if IsGenericImazingAttachmentName(attachmentFilename) {
			attachmentData = append([]byte(nil), imazingPlaceholderPNG...)
			attachmentFilename = "imazing-placeholder.png"
			attachmentType = "image/png"
			stats.mu.Lock()
			stats.AttachmentsFound++
			stats.PlaceholdersUsed++
			stats.mu.Unlock()
		} else {
			filePath, actualFilename, err := utils.FindAttachmentFileWithFallback(csvDir, attachmentFilename)
			if err == nil {
				data, err := os.ReadFile(filePath)
				if err == nil {
					attachmentData = data
					stats.mu.Lock()
					stats.AttachmentsFound++
					stats.mu.Unlock()

					if usedBasenames != nil {
						usedBasenames[filepath.Base(filePath)] = struct{}{}
					}

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
