package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

const personalScoreBodyRunes = 4000

// ErrPersonalScoreRunning is returned when this user already has a scoring job in progress.
var ErrPersonalScoreRunning = errors.New("a personal score job is already running")

// EmailPersonalScoreEvent is one progress snapshot for the scoring job.
type EmailPersonalScoreEvent struct {
	Type        string `json:"type"`
	Running     bool   `json:"running"`
	Finished    bool   `json:"finished"`
	Total       int    `json:"total"`
	Done        int    `json:"done"`
	Failed      int    `json:"failed"`
	Current     int    `json:"current"`
	From        string `json:"from"`
	Subject     string `json:"subject"`
	Body        string `json:"body"`
	Score       *int   `json:"score"`
	JevResponse string `json:"jev_response"`
	Error       string `json:"error"`
	Cancelled   bool   `json:"cancelled"`
}

type personalScoreJob struct {
	cancel    context.CancelFunc
	running   bool
	finished  bool
	cancelled bool
	total     int
	done      int
	failed    int
	current   int
	from      string
	subject   string
	body      string
	jev       string
	errMsg    string
	score     *int
	subs      map[int]chan EmailPersonalScoreEvent
	nextSub   int
}

func (j *personalScoreJob) event(kind string) EmailPersonalScoreEvent {
	var score *int
	if j.score != nil {
		v := *j.score
		score = &v
	}
	return EmailPersonalScoreEvent{
		Type:        kind,
		Running:     j.running,
		Finished:    j.finished,
		Total:       j.total,
		Done:        j.done,
		Failed:      j.failed,
		Current:     j.current,
		From:        j.from,
		Subject:     j.subject,
		Body:        j.body,
		Score:       score,
		JevResponse: j.jev,
		Error:       j.errMsg,
		Cancelled:   j.cancelled,
	}
}

func (s *EmailService) ensureScoreJobs() {
	if s.scoreJobs == nil {
		s.scoreJobs = map[int64]*personalScoreJob{}
	}
}

// StartPersonalScore scores unscored emails, or every non-deleted email when rescoreAll is set.
// apiKey is the archive OpenRouter key captured on the request that started the job.
func (s *EmailService) StartPersonalScore(uid int64, apiKey string, rescoreAll bool) (EmailPersonalScoreEvent, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return EmailPersonalScoreEvent{}, errors.New("OpenRouter API key is not configured")
	}
	s.scoreMu.Lock()
	s.ensureScoreJobs()
	if j := s.scoreJobs[uid]; j != nil && j.running {
		ev := j.event("status")
		s.scoreMu.Unlock()
		return ev, ErrPersonalScoreRunning
	}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), appctx.ContextKeyUserID, uid))
	job := &personalScoreJob{
		cancel:  cancel,
		running: true,
		subs:    map[int]chan EmailPersonalScoreEvent{},
	}
	s.scoreJobs[uid] = job
	ev := job.event("status")
	s.scoreMu.Unlock()
	go s.runPersonalScore(ctx, uid, apiKey, rescoreAll)
	return ev, nil
}

// PersonalScoreStatus returns the latest snapshot for this user's job.
func (s *EmailService) PersonalScoreStatus(uid int64) EmailPersonalScoreEvent {
	s.scoreMu.Lock()
	defer s.scoreMu.Unlock()
	j := s.scoreJobs[uid]
	if j == nil {
		return EmailPersonalScoreEvent{Type: "idle"}
	}
	return j.event("status")
}

// SubscribePersonalScore registers a live listener. Closing the subscription does not stop the job.
func (s *EmailService) SubscribePersonalScore(uid int64) (<-chan EmailPersonalScoreEvent, func()) {
	s.scoreMu.Lock()
	defer s.scoreMu.Unlock()
	j := s.scoreJobs[uid]
	if j == nil {
		return nil, func() {}
	}
	ch := make(chan EmailPersonalScoreEvent, 32)
	id := j.nextSub
	j.nextSub++
	if j.subs == nil {
		j.subs = map[int]chan EmailPersonalScoreEvent{}
	}
	j.subs[id] = ch
	kind := "status"
	if j.finished {
		kind = "done"
	}
	ch <- j.event(kind)
	job := j
	unsub := func() {
		s.scoreMu.Lock()
		defer s.scoreMu.Unlock()
		delete(job.subs, id)
	}
	return ch, unsub
}

// CancelPersonalScore stops the running job for this user. It does not clear stored scores.
func (s *EmailService) CancelPersonalScore(uid int64) {
	s.scoreMu.Lock()
	j := s.scoreJobs[uid]
	var cancel context.CancelFunc
	if j != nil {
		cancel = j.cancel
	}
	s.scoreMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *EmailService) runPersonalScore(ctx context.Context, uid int64, apiKey string, rescoreAll bool) {
	ids, err := s.repo.ListPersonalScoreCandidateIDs(ctx, rescoreAll)
	if err != nil {
		s.finishPersonalScore(uid, ctx.Err() != nil, err.Error())
		return
	}
	s.updatePersonalScore(uid, func(j *personalScoreJob) {
		j.total = len(ids)
		j.errMsg = ""
	}, "status")
	for i, id := range ids {
		if ctx.Err() != nil {
			break
		}
		s.scoreOneEmail(ctx, uid, apiKey, id, i+1)
	}
	s.finishPersonalScore(uid, ctx.Err() != nil, "")
}

func (s *EmailService) scoreOneEmail(ctx context.Context, uid int64, apiKey string, id int64, current int) {
	src, err := s.repo.GetPersonalScoreSource(ctx, id)
	if ctx.Err() != nil {
		return
	}
	if err != nil || src == nil {
		msg := "email not found"
		if err != nil {
			msg = err.Error()
		}
		s.updatePersonalScore(uid, func(j *personalScoreJob) {
			j.current = current
			j.done++
			j.failed++
			j.score = nil
			j.from, j.subject, j.body, j.jev = "", "", "", ""
			j.errMsg = msg
		}, "step")
		return
	}
	body := personalScoreBody(src)
	from := derefString(src.FromAddress)
	subject := derefString(src.Subject)
	s.updatePersonalScore(uid, func(j *personalScoreJob) {
		j.current = current
	}, "status")
	state := map[string]any{
		"subject": subject,
		"from":    from,
		"to":      derefString(src.ToAddresses),
		"date":    formatPersonalScoreDate(src.Date),
		"body":    body,
	}
	questions := map[string]any{
		"specifically_personal": map[string]any{
			"type":         "noul",
			"instructions": "Is this email specifically personal?",
			"criteria": map[string]string{
				"false": "Spam, marketing, newsletters, or other bulk mail not written for one person.",
				"true":  "Written to or about a specific person.",
			},
		},
		"business": map[string]any{
			"type":         "noul",
			"instructions": "Is this email business correspondence?",
			"criteria": map[string]string{
				"false": "Personal mail, spam, marketing, or newsletters.",
				"true":  "Work, commercial, or organizational mail about a real matter.",
			},
		},
		"important": map[string]any{
			"type":         "noul",
			"instructions": "Is this email important?",
			"criteria": map[string]string{
				"false": "Routine, bulk, or low-value mail.",
				"true":  "The recipient would reasonably need to act on it, reply, or keep it.",
			},
		},
	}
	result, raw, callErr := ai.CallJevDecisions(ctx, apiKey, state, questions)
	if ctx.Err() != nil {
		return
	}
	rawText := formatJevRaw(raw)
	if callErr != nil {
		s.updatePersonalScore(uid, func(j *personalScoreJob) {
			j.current = current
			j.done++
			j.failed++
			j.score = nil
			j.from, j.subject, j.body, j.jev = from, subject, body, rawText
			j.errMsg = callErr.Error()
		}, "step")
		return
	}
	personal, okPersonal := noulAnswer(result, "specifically_personal")
	business, okBusiness := noulAnswer(result, "business")
	important, okImportant := noulAnswer(result, "important")
	if !okPersonal && !okBusiness && !okImportant {
		s.updatePersonalScore(uid, func(j *personalScoreJob) {
			j.current = current
			j.done++
			j.failed++
			j.score = nil
			j.from, j.subject, j.body, j.jev = from, subject, body, rawText
			j.errMsg = "Jev did not return a probability"
		}, "step")
		return
	}
	var scorePtr *int
	var isPersonal, isBusiness, isImportant *bool
	var saveErr error
	if okPersonal {
		score := personalScoreFromNoul(personal)
		scorePtr = &score
		if err := s.repo.SetPersonalScore(ctx, id, score); err != nil {
			saveErr = err
		} else {
			flag := emailClassFlag(personal)
			isPersonal = &flag
		}
	}
	if ctx.Err() != nil {
		return
	}
	if okBusiness {
		flag := emailClassFlag(business)
		isBusiness = &flag
	}
	if okImportant {
		flag := emailClassFlag(important)
		isImportant = &flag
	}
	if isPersonal != nil || isBusiness != nil || isImportant != nil {
		if _, err := s.repo.Update(ctx, id, isPersonal, isBusiness, isImportant, nil); err != nil && saveErr == nil {
			saveErr = err
		}
	}
	if ctx.Err() != nil {
		return
	}
	errMsg := ""
	if saveErr != nil {
		errMsg = saveErr.Error()
	} else if !okPersonal {
		errMsg = "Jev did not return a personal probability"
	}
	s.updatePersonalScore(uid, func(j *personalScoreJob) {
		j.current = current
		j.done++
		if errMsg != "" {
			j.failed++
		}
		j.score = scorePtr
		j.from, j.subject, j.body, j.jev = from, subject, body, rawText
		j.errMsg = errMsg
	}, "step")
}

func (s *EmailService) finishPersonalScore(uid int64, cancelled bool, errMsg string) {
	s.updatePersonalScore(uid, func(j *personalScoreJob) {
		j.running = false
		j.finished = true
		j.cancelled = cancelled
		if errMsg != "" && !cancelled {
			j.errMsg = errMsg
		}
	}, "done")
}

func (s *EmailService) updatePersonalScore(uid int64, fn func(*personalScoreJob), kind string) {
	s.scoreMu.Lock()
	j := s.scoreJobs[uid]
	if j == nil {
		s.scoreMu.Unlock()
		return
	}
	fn(j)
	ev := j.event(kind)
	chans := make([]chan EmailPersonalScoreEvent, 0, len(j.subs))
	for _, ch := range j.subs {
		chans = append(chans, ch)
	}
	s.scoreMu.Unlock()
	for _, ch := range chans {
		select {
		case ch <- ev:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

func personalScoreBody(src *model.EmailPersonalScoreSource) string {
	text := strings.TrimSpace(derefString(src.PlainText))
	if text == "" {
		text = strings.TrimSpace(derefString(src.Snippet))
	}
	return truncatePersonalScore(text, personalScoreBodyRunes)
}

func formatPersonalScoreDate(d sqlutil.NullDBTime) string {
	if !d.Valid {
		return ""
	}
	return d.Time.UTC().Format(time.RFC3339)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncatePersonalScore(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}

// emailClassFlag is true when a Jev yes/no probability is over one half.
func emailClassFlag(noul float64) bool {
	return noul > 0.5
}

func personalScoreFromNoul(p float64) int {
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	return int(math.Round(p * 100))
}

func noulAnswer(result *ai.JevDecisionResult, key string) (float64, bool) {
	if result == nil || result.Answers == nil {
		return 0, false
	}
	raw, ok := result.Answers[key]
	if !ok {
		return 0, false
	}
	var ans struct {
		Noul *float64 `json:"noul"`
	}
	if err := json.Unmarshal(raw, &ans); err != nil || ans.Noul == nil {
		return 0, false
	}
	return *ans.Noul, true
}

func formatJevRaw(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	const max = 100_000
	if len(raw) > max {
		raw = raw[:max]
	}
	var buf any
	if err := json.Unmarshal(raw, &buf); err != nil {
		return string(raw)
	}
	pretty, err := json.MarshalIndent(buf, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(pretty)
}
