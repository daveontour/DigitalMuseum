package contacts

import (
	"sort"
	"strings"
)

const FuzzyMergeThreshold = 0.85

type similarityTask struct {
	GroupID int
	A, B    string
}

type similarityResult struct {
	GroupID int
	Score   float64
}

var commonNameVariations = map[string][]string{
	"dave": {"david", "dave"}, "david": {"dave", "david"},
	"bob": {"robert", "bob"}, "robert": {"bob", "robert"},
	"bill": {"william", "bill"}, "william": {"bill", "william"},
	"jim": {"james", "jim"}, "james": {"jim", "james"},
	"mike": {"michael", "mike"}, "michael": {"mike", "michael"},
	"tom": {"thomas", "tom"}, "thomas": {"tom", "thomas"},
	"chris": {"christopher", "chris"}, "christopher": {"chris", "christopher"},
	"elle": {"ellen", "ellen"}, "ellie": {"elle", "ellie"},
	"emma": {"emily", "emma"}, "emily": {"emma", "emily"},
	"ella": {"ellie", "ella"},
	"elizabeth": {"liz", "elizabeth"}, "liz": {"elizabeth", "liz"},
	"sarah": {"sarah", "sarah"},
}

func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersect := 0
	for k := range a {
		if _, ok := b[k]; ok {
			intersect++
		}
	}
	return float64(intersect) / float64(len(a)+len(b)-intersect)
}

func levenshteinRatio(a, b string) float64 {
	ar := []rune(a)
	br := []rune(b)
	dp := make([][]int, len(ar)+1)
	for i := range dp {
		dp[i] = make([]int, len(br)+1)
	}
	for i := range ar {
		dp[i+1][0] = i + 1
	}
	for j := range br {
		dp[0][j+1] = j + 1
	}
	for i := range ar {
		for j := range br {
			cost := 0
			if ar[i] != br[j] {
				cost = 1
			}
			dp[i+1][j+1] = minInt3(dp[i][j+1]+1, dp[i+1][j]+1, dp[i][j]+cost)
		}
	}
	dist := dp[len(ar)][len(br)]
	maxLen := len(ar)
	if len(br) > maxLen {
		maxLen = len(br)
	}
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - float64(dist)/float64(maxLen)
}

func minInt3(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}

func initialsMatch(a, b string) float64 {
	at := strings.Split(a, " ")
	bt := strings.Split(b, " ")
	if len(at) >= 2 && len(bt) >= 2 {
		if at[0][0] == bt[0][0] && at[len(at)-1] == bt[len(bt)-1] {
			return 1.0
		}
	}
	return 0.0
}

func checkNameVariations(a, b string) float64 {
	at := strings.Split(a, " ")
	bt := strings.Split(b, " ")
	if len(at) >= 2 && len(bt) >= 2 {
		if at[len(at)-1] != bt[len(bt)-1] {
			return 0.0
		}
		firstA := strings.ToLower(at[0])
		firstB := strings.ToLower(bt[0])
		if firstA == firstB {
			return 1.0
		}
		if variations, ok := commonNameVariations[firstA]; ok {
			for _, v := range variations {
				if v == firstB {
					return 0.95
				}
			}
		}
		if variations, ok := commonNameVariations[firstB]; ok {
			for _, v := range variations {
				if v == firstA {
					return 0.95
				}
			}
		}
	}
	return 0.0
}

func fuzzySimilarity(a, b string) float64 {
	if variationScore := checkNameVariations(a, b); variationScore > 0 {
		return variationScore
	}
	at := strings.Fields(a)
	bt := strings.Fields(b)
	if len(at) >= 2 && len(bt) >= 2 {
		if strings.ToLower(at[len(at)-1]) != strings.ToLower(bt[len(bt)-1]) {
			return 0.0
		}
	}
	return 0.45*levenshteinRatio(a, b) +
		0.45*jaccard(tokenize(a), tokenize(b)) +
		0.10*initialsMatch(a, b)
}

func similarityWorker(tasks <-chan similarityTask, results chan<- similarityResult) {
	for t := range tasks {
		results <- similarityResult{GroupID: t.GroupID, Score: fuzzySimilarity(t.A, t.B)}
	}
}

func choosePrimaryName(freq map[string]int) string {
	if len(freq) == 0 {
		return "Unknown"
	}
	type kv struct {
		Name  string
		Count int
	}
	var arr []kv
	for k, v := range freq {
		arr = append(arr, kv{k, v})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].Count == arr[j].Count {
			return len(arr[i].Name) < len(arr[j].Name)
		}
		return arr[i].Count > arr[j].Count
	})
	primary := arr[0].Name
	if isEmailAddress(primary) {
		for _, item := range arr {
			if !isEmailAddress(item.Name) {
				primary = item.Name
				break
			}
		}
	}
	return capitalizeName(primary)
}

// CreateEmailMap builds email->ID map from formatted output
func CreateEmailMap(formattedOutput []FormattedOutputRecord) map[string]int {
	emailMap := make(map[string]int)
	for _, record := range formattedOutput {
		emails := strings.Split(record.Emails, ",")
		for _, email := range emails {
			email = strings.TrimSpace(email)
			email = strings.ReplaceAll(email, `"`, "")
			email = strings.ReplaceAll(email, "<", "")
			email = strings.ReplaceAll(email, ">", "")
			email = strings.ToLower(email)
			emailMap[email] = record.ID
		}
	}
	return emailMap
}
