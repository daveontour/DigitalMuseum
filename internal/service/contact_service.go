package service

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// ContactService manages contacts and related tables.
type ContactService struct {
	repo       *repository.ContactRepo
	subjectCfg *repository.SubjectConfigRepo
}

// NewContactService creates a ContactService.
func NewContactService(repo *repository.ContactRepo, subjectCfg *repository.SubjectConfigRepo) *ContactService {
	return &ContactService{repo: repo, subjectCfg: subjectCfg}
}

func (s *ContactService) ownerArchiveName(ctx context.Context) string {
	if s.subjectCfg == nil {
		return "Subject"
	}
	cfg, err := s.subjectCfg.GetFirst(ctx)
	if err != nil || cfg == nil {
		return "Subject"
	}
	first := strings.TrimSpace(cfg.SubjectName)
	family := ""
	if cfg.FamilyName != nil {
		family = strings.TrimSpace(*cfg.FamilyName)
	}
	full := strings.TrimSpace(strings.Join([]string{first, family}, " "))
	if full != "" {
		return full
	}
	return "Subject"
}

// ── Contacts ──────────────────────────────────────────────────────────────────

func (s *ContactService) ListShort(ctx context.Context, p repository.ContactListParams) ([]*model.Contact, int, error) {
	return s.repo.ListShort(ctx, p)
}

func (s *ContactService) ListNames(ctx context.Context) ([]struct {
	ID   int64
	Name string
}, error) {
	return s.repo.ListNames(ctx)
}

func (s *ContactService) Delete(ctx context.Context, id int64) (bool, error) {
	return s.repo.Delete(ctx, id)
}

func (s *ContactService) BulkDelete(ctx context.Context, ids []int64) (deleted, skipped []int64, err error) {
	return s.repo.BulkDelete(ctx, ids)
}

// UpdateClassification finds contacts matching name and sets their rel_type.
// It also applies to the email_classifications table if a row exists.
func (s *ContactService) UpdateClassification(ctx context.Context, name, classification string) error {
	if err := s.repo.ApplyClassificationToContacts(ctx, name, classification); err != nil {
		return fmt.Errorf("updateClassification: %w", err)
	}
	return nil
}

// ── Relationship graph ────────────────────────────────────────────────────────

// RelationshipGraph holds the graph nodes and links for the frontend.
type RelationshipGraph struct {
	Nodes []RelGraphNode `json:"nodes"`
	Links []RelGraphLink `json:"links"`
}

type RelGraphNode struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	ContactType  *string `json:"contact_type"`
	NumEmails    *int    `json:"num_emails"`
	NumIMessages *int    `json:"num_imessages"`
	NumFacebook  *int    `json:"num_facebook"`
	NumWhatsApp  *int    `json:"num_whatsapp"`
	NumSMS       *int    `json:"num_sms"`
	NumInstagram *int    `json:"num_instagram"`
}

type RelGraphLink struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Strength int    `json:"strength"`
}

// relGraphSubjectNodeID is the cytoscape hub node for the archive subject (never use "0" — falsy/coercion issues).
const relGraphSubjectNodeID = "__subject__"

func (s *ContactService) GetRelationshipGraph(ctx context.Context, types, sources []string, maxNodes int) (*RelationshipGraph, error) {
	rows, err := s.repo.GetRelationshipGraph(ctx, types, sources, maxNodes)
	if err != nil {
		return nil, err
	}

	// Build nodes
	var nodes []RelGraphNode
	subjectName := s.ownerArchiveName(ctx)
	nodeTotals := map[string]int64{}

	for _, c := range rows {
		displayName := strings.TrimSpace(c.Name)
		var nodeID string
		if c.ID == 0 {
			nodeID = relGraphSubjectNodeID
			if displayName != "" && subjectName == "Subject" {
				subjectName = displayName
			}
			if displayName == "" {
				displayName = subjectName
			}
		} else {
			nodeID = fmt.Sprintf("c:%d", c.ID)
			if displayName == "" {
				displayName = fmt.Sprintf("Contact %d", c.ID)
			}
			nodeTotals[nodeID] = c.Total
		}
		nodes = append(nodes, RelGraphNode{
			ID:           nodeID,
			Name:         displayName,
			ContactType:  c.RelType,
			NumEmails:    c.NumEmails,
			NumIMessages: c.NumIMessages,
			NumFacebook:  c.NumFacebook,
			NumWhatsApp:  c.NumWhatsApp,
			NumSMS:       c.NumSMS,
			NumInstagram: c.NumInstagram,
		})
	}

	// Dedupe by graph id (same contact should not appear twice).
	seenIDs := make(map[string]struct{}, len(nodes))
	deduped := make([]RelGraphNode, 0, len(nodes))
	for _, n := range nodes {
		if _, ok := seenIDs[n.ID]; ok {
			continue
		}
		seenIDs[n.ID] = struct{}{}
		deduped = append(deduped, n)
	}
	nodes = deduped

	// Ensure subject hub exists and is first.
	subjectIdx := -1
	for i, n := range nodes {
		if n.ID == relGraphSubjectNodeID {
			subjectIdx = i
			break
		}
	}
	if subjectIdx < 0 {
		nodes = append([]RelGraphNode{{ID: relGraphSubjectNodeID, Name: subjectName}}, nodes...)
	} else if subjectIdx > 0 {
		subject := nodes[subjectIdx]
		nodes = append([]RelGraphNode{subject}, append(nodes[:subjectIdx], nodes[subjectIdx+1:]...)...)
	}

	nodeIDs := make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		nodeIDs[n.ID] = struct{}{}
	}

	// Build links with log-scaled strength
	maxRaw := int64(1)
	for _, t := range nodeTotals {
		if t > maxRaw {
			maxRaw = t
		}
	}
	lnMax := math.Log(float64(maxRaw) + 1)

	var links []RelGraphLink
	for _, n := range nodes {
		if n.ID == relGraphSubjectNodeID {
			continue
		}
		raw := nodeTotals[n.ID]
		strength := 1
		if lnMax > 0 {
			strength = int(math.Round(1 + 9*math.Log(float64(raw)+1)/lnMax))
		}
		if strength < 1 {
			strength = 1
		}
		if strength > 10 {
			strength = 10
		}
		links = append(links, RelGraphLink{
			Source:   n.ID,
			Target:   relGraphSubjectNodeID,
			Strength: strength,
		})
	}
	// Drop any edge whose endpoint is missing (defensive; should not happen after ensure above).
	filtered := links[:0]
	for _, l := range links {
		if _, ok := nodeIDs[l.Source]; !ok {
			continue
		}
		if _, ok := nodeIDs[l.Target]; !ok {
			continue
		}
		filtered = append(filtered, l)
	}
	links = filtered

	return &RelationshipGraph{Nodes: nodes, Links: links}, nil
}

// ── Email matches ─────────────────────────────────────────────────────────────

func (s *ContactService) ListEmailMatches(ctx context.Context, primaryName string) ([]*model.EmailMatch, error) {
	return s.repo.ListEmailMatches(ctx, primaryName)
}

func (s *ContactService) GetEmailMatchByID(ctx context.Context, id int64) (*model.EmailMatch, error) {
	return s.repo.GetEmailMatchByID(ctx, id)
}

func (s *ContactService) CreateEmailMatch(ctx context.Context, primaryName, email string) (*model.EmailMatch, error) {
	exists, err := s.repo.EmailMatchExists(ctx, primaryName, email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("conflict:email match already exists for %s / %s", primaryName, email)
	}
	return s.repo.CreateEmailMatch(ctx, primaryName, email)
}

func (s *ContactService) UpdateEmailMatch(ctx context.Context, id int64, primaryName, email *string) (*model.EmailMatch, error) {
	return s.repo.UpdateEmailMatch(ctx, id, primaryName, email)
}

func (s *ContactService) DeleteEmailMatch(ctx context.Context, id int64) (bool, error) {
	return s.repo.DeleteEmailMatch(ctx, id)
}

// ── Email exclusions ──────────────────────────────────────────────────────────

func (s *ContactService) ListEmailExclusions(ctx context.Context, search string, nameEmail *bool) ([]*model.EmailExclusion, error) {
	return s.repo.ListEmailExclusions(ctx, search, nameEmail)
}

func (s *ContactService) GetEmailExclusionByID(ctx context.Context, id int64) (*model.EmailExclusion, error) {
	return s.repo.GetEmailExclusionByID(ctx, id)
}

func (s *ContactService) CreateEmailExclusion(ctx context.Context, email, name string, nameEmail bool) (*model.EmailExclusion, error) {
	exists, err := s.repo.ExclusionExists(ctx, email, name, nameEmail)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("conflict:email exclusion already exists")
	}
	return s.repo.CreateEmailExclusion(ctx, email, name, nameEmail)
}

func (s *ContactService) UpdateEmailExclusion(ctx context.Context, id int64, email, name *string, nameEmail *bool) (*model.EmailExclusion, error) {
	return s.repo.UpdateEmailExclusion(ctx, id, email, name, nameEmail)
}

func (s *ContactService) DeleteEmailExclusion(ctx context.Context, id int64) (bool, error) {
	return s.repo.DeleteEmailExclusion(ctx, id)
}

// ── Email classifications ─────────────────────────────────────────────────────

func (s *ContactService) ListEmailClassifications(ctx context.Context, name, classification string) ([]*model.EmailClassification, error) {
	return s.repo.ListEmailClassifications(ctx, name, classification)
}

func (s *ContactService) GetEmailClassificationByID(ctx context.Context, id int64) (*model.EmailClassification, error) {
	return s.repo.GetEmailClassificationByID(ctx, id)
}

func (s *ContactService) CreateEmailClassification(ctx context.Context, name, classification string) (*model.EmailClassification, error) {
	exists, err := s.repo.ClassificationExists(ctx, name, classification)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("conflict:classification already exists for %s / %s", name, classification)
	}
	return s.repo.CreateEmailClassification(ctx, name, classification)
}

func (s *ContactService) UpdateEmailClassification(ctx context.Context, id int64, name, classification *string) (*model.EmailClassification, error) {
	return s.repo.UpdateEmailClassification(ctx, id, name, classification)
}

func (s *ContactService) DeleteEmailClassification(ctx context.Context, id int64) (bool, error) {
	return s.repo.DeleteEmailClassification(ctx, id)
}
