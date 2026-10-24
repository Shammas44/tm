package tui

import (
	"strings"
	"taskmanager/internal/model"
)

type FilterCriteria struct {
	Text         string
	IncludedTags []string
	ExcludedTags []string
	Project      string
	Priority     string
	Status       string
}

func ParseSearchQuery(query string) FilterCriteria {
	var c FilterCriteria
	tokens := strings.Fields(query)

	var textTokens []string
	for _, tok := range tokens {
		tokLower := strings.ToLower(tok)
		if strings.HasPrefix(tok, "+") && len(tok) > 1 {
			c.IncludedTags = append(c.IncludedTags, strings.ToUpper(tok[1:]))
		} else if strings.HasPrefix(tok, "-") && len(tok) > 1 {
			c.ExcludedTags = append(c.ExcludedTags, strings.ToUpper(tok[1:]))
		} else if strings.HasPrefix(tokLower, "project:") || strings.HasPrefix(tokLower, "p:") {
			parts := strings.SplitN(tok, ":", 2)
			c.Project = strings.ToLower(parts[1])
		} else if strings.HasPrefix(tokLower, "priority:") || strings.HasPrefix(tokLower, "pri:") {
			parts := strings.SplitN(tok, ":", 2)
			c.Priority = strings.ToUpper(parts[1])
		} else if strings.HasPrefix(tokLower, "status:") || strings.HasPrefix(tokLower, "s:") {
			parts := strings.SplitN(tok, ":", 2)
			c.Status = strings.ToLower(parts[1])
		} else {
			textTokens = append(textTokens, tok)
		}
	}
	c.Text = strings.ToLower(strings.Join(textTokens, " "))
	return c
}

func MatchesFilter(t *model.Task, allTasks []*model.Task, hasNotes bool, c FilterCriteria) bool {
	// 1. Text search across title, description, blocker reason
	if c.Text != "" {
		match := strings.Contains(strings.ToLower(t.Title), c.Text) ||
			strings.Contains(strings.ToLower(t.Description), c.Text) ||
			strings.Contains(strings.ToLower(t.Blocker), c.Text) ||
			strings.Contains(strings.ToLower(t.Project), c.Text)
		if !match {
			return false
		}
	}

	// 2. Project filter
	if c.Project != "" && !strings.Contains(strings.ToLower(t.Project), c.Project) {
		return false
	}

	// 3. Priority filter
	if c.Priority != "" && string(t.Priority) != c.Priority {
		return false
	}

	// 4. Status filter
	if c.Status != "" {
		sLower := strings.ToLower(string(t.Status))
		if c.Status == "active" {
			c.Status = "in_progress"
		}
		if sLower != c.Status {
			return false
		}
	}

	// 5. Tags & Virtual Tags
	if len(c.IncludedTags) > 0 || len(c.ExcludedTags) > 0 {
		allTags := t.AllTags(allTasks, hasNotes)
		tagSet := make(map[string]bool)
		for _, tag := range allTags {
			tagSet[strings.ToUpper(tag)] = true
		}

		for _, inc := range c.IncludedTags {
			if !tagSet[inc] {
				return false
			}
		}

		for _, exc := range c.ExcludedTags {
			if tagSet[exc] {
				return false
			}
		}
	}

	return true
}
