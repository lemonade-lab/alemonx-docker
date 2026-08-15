package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Recommendation is one parsed entry from recommendations.md. Contributors
// edit that Markdown file directly in the repository; the runner only parses
// and validates it. Each entry downloads either from a public HTTPS URL or
// from a bundled example file inside the plugin directory.
type Recommendation struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	URL         string   `json:"url,omitempty"`
	Example     string   `json:"example,omitempty"`
}

// recommendationsFile lives at the plugin root next to alx.json and ships in
// the release bundle. Tests override it with a temporary file.
var recommendationsFile = "recommendations.md"

// loadRecommendations reads recommendations.md from the plugin directory (the
// runner working directory, with a fallback to the executable directory).
func loadRecommendations() ([]Recommendation, error) {
	data, err := os.ReadFile(recommendationsFile)
	if err != nil && !filepath.IsAbs(recommendationsFile) {
		if executable, execErr := os.Executable(); execErr == nil {
			data, err = os.ReadFile(filepath.Join(filepath.Dir(executable), recommendationsFile))
		}
	}
	if err != nil {
		return nil, fmt.Errorf("无法读取推荐清单：%w", err)
	}
	return parseRecommendations(string(data))
}

// parseRecommendations turns the Markdown recommendation file into a list.
// A `## 名称` heading starts an entry; key-value bullets (`- 键：值` or
// `- 键: 值`) fill 描述/标签/地址/示例. Comments and prose are ignored, and
// entries without a name or without any download source are dropped.
func parseRecommendations(markdown string) ([]Recommendation, error) {
	items := []Recommendation{}
	current := -1
	inComment := false
	for _, raw := range strings.Split(markdown, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if inComment {
			if strings.Contains(line, "-->") {
				inComment = false
			}
			continue
		}
		if strings.HasPrefix(line, "<!--") {
			if !strings.Contains(line, "-->") {
				inComment = true
			}
			continue
		}
		if strings.HasPrefix(line, "## ") {
			items = append(items, Recommendation{Name: strings.TrimSpace(strings.TrimPrefix(line, "## "))})
			current = len(items) - 1
			continue
		}
		if strings.HasPrefix(line, "#") || current < 0 {
			continue
		}
		bullet := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "-"), "*"))
		key, value, found := strings.Cut(bullet, "：")
		if !found {
			key, value, found = strings.Cut(bullet, ":")
		}
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		item := &items[current]
		switch key {
		case "描述", "description":
			item.Description = value
		case "标签", "tags":
			item.Tags = splitTags(value)
		case "地址", "url":
			if strings.HasPrefix(strings.ToLower(value), "https://") {
				item.URL = value
			}
		case "示例", "example":
			item.Example = value
		}
	}

	valid := make([]Recommendation, 0, len(items))
	seen := map[string]int{}
	for _, item := range items {
		if item.Name == "" || (item.URL == "" && item.Example == "") {
			continue
		}
		id := recommendationID(item.Name)
		if seen[id] > 0 {
			seen[id]++
			id = fmt.Sprintf("%s-%d", id, seen[id])
		} else {
			seen[id] = 1
		}
		item.ID = id
		valid = append(valid, item)
	}
	return valid, nil
}

func splitTags(value string) []string {
	tags := []string{}
	for _, tag := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' }) {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func recommendationID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var builder strings.Builder
	lastDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	id := strings.Trim(builder.String(), "-")
	if id == "" {
		id = "recommendation"
	}
	if id[0] >= '0' && id[0] <= '9' {
		id = "rec-" + id
	}
	if len(id) > 48 {
		id = strings.TrimRight(id[:48], "-")
	}
	return id
}

// importExampleProject copies a bundled example compose file from the plugin
// directory into the managed project library. The path must stay inside the
// plugin directory; size and Compose validation reuse the shared limits.
func importExampleProject(name, example string) (Project, error) {
	if strings.TrimSpace(example) == "" {
		return Project{}, errors.New("缺少示例路径")
	}
	clean := filepath.Clean(filepath.FromSlash(example))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return Project{}, errors.New("示例路径无效")
	}
	dir, err := os.Getwd()
	if err != nil {
		return Project{}, err
	}
	data, err := readLimitedFile(filepath.Join(dir, clean))
	if err != nil {
		return Project{}, fmt.Errorf("无法读取示例文件：%w", err)
	}
	return createProjectWithContent(name, "示例："+clean, string(data))
}
